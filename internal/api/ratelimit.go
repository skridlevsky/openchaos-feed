package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements sliding window rate limiting
type RateLimiter struct {
	mu       sync.RWMutex
	windows  map[string]*slidingWindow
	limit    int
	window   time.Duration
	keyFunc  func(r *http.Request) string
	cleanupT *time.Ticker
	stopCh   chan struct{}
	stopOnce sync.Once
}

// slidingWindow tracks requests in a sliding time window
type slidingWindow struct {
	timestamps []time.Time
	mu         sync.Mutex
}

// RateLimitConfig defines rate limit parameters
type RateLimitConfig struct {
	Limit   int           // Max requests per window
	Window  time.Duration // Time window
	KeyFunc func(r *http.Request) string
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = GetClientIP
	}

	rl := &RateLimiter{
		windows: make(map[string]*slidingWindow),
		limit:   cfg.Limit,
		window:  cfg.Window,
		keyFunc: cfg.KeyFunc,
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine to prevent memory leaks
	rl.cleanupT = time.NewTicker(cfg.Window)
	go rl.cleanup()

	return rl
}

// cleanup periodically removes expired entries
func (rl *RateLimiter) cleanup() {
	for {
		select {
		case <-rl.cleanupT.C:
			rl.mu.Lock()
			now := time.Now()
			for key, sw := range rl.windows {
				sw.mu.Lock()
				sw.pruneOld(now, rl.window)
				if len(sw.timestamps) == 0 {
					delete(rl.windows, key)
				}
				sw.mu.Unlock()
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			rl.cleanupT.Stop()
			return
		}
	}
}

// Stop stops the cleanup goroutine. Safe to call multiple times.
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(r *http.Request) bool {
	key := rl.keyFunc(r)
	now := time.Now()

	rl.mu.Lock()
	sw, exists := rl.windows[key]
	if !exists {
		sw = &slidingWindow{}
		rl.windows[key] = sw
	}
	rl.mu.Unlock()

	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Remove old timestamps outside the window
	sw.pruneOld(now, rl.window)

	// Check if under limit
	if len(sw.timestamps) >= rl.limit {
		return false
	}

	// Add current timestamp
	sw.timestamps = append(sw.timestamps, now)
	return true
}

// pruneOld removes timestamps older than the window
func (sw *slidingWindow) pruneOld(now time.Time, window time.Duration) {
	cutoff := now.Add(-window)
	i := 0
	for i < len(sw.timestamps) && sw.timestamps[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		sw.timestamps = sw.timestamps[i:]
	}
}

// Middleware returns HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(r) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetClientIP extracts the client IP from a request.
// chi middleware.RealIP already sets r.RemoteAddr from X-Real-IP / X-Forwarded-For,
// so we only need to strip the port. Do NOT re-read those headers here — an attacker
// can spoof X-Forwarded-For to bypass per-IP rate limits.
// Uses net.SplitHostPort to correctly handle both IPv4 and IPv6 addresses.
func GetClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr may not have a port (e.g. unix socket)
		return r.RemoteAddr
	}
	return host
}

// RateLimiters holds all rate limiters for the application
type RateLimiters struct {
	Global *RateLimiter
	Export *RateLimiter
}

// NewRateLimiters creates the standard rate limiters
func NewRateLimiters() *RateLimiters {
	return &RateLimiters{
		// Global: 100 requests per minute per IP
		Global: NewRateLimiter(RateLimitConfig{
			Limit:   100,
			Window:  1 * time.Minute,
			KeyFunc: GetClientIP,
		}),
		// Export: 2 requests per minute per IP (heavy endpoint)
		Export: NewRateLimiter(RateLimitConfig{
			Limit:   2,
			Window:  1 * time.Minute,
			KeyFunc: GetClientIP,
		}),
	}
}

// Stop stops all rate limiter cleanup goroutines
func (rls *RateLimiters) Stop() {
	rls.Global.Stop()
	rls.Export.Stop()
}

// ExportSemaphore limits concurrent export operations to prevent
// DB connection pool exhaustion. Max 3 concurrent exports system-wide.
var ExportSemaphore = make(chan struct{}, 3)

// ExportGuardMiddleware applies both the strict export rate limit and
// the concurrency semaphore. Returns 429 if rate limited, 503 if all
// export slots are in use.
func ExportGuardMiddleware(exportRL *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Layer 1: Per-IP rate limit
			if !exportRL.Allow(r) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "Export rate limit exceeded (max 2/min)", http.StatusTooManyRequests)
				return
			}

			// Layer 2: Global concurrency semaphore (non-blocking)
			select {
			case ExportSemaphore <- struct{}{}:
				defer func() { <-ExportSemaphore }()
			default:
				http.Error(w, "Export capacity full, try again shortly", http.StatusServiceUnavailable)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
