package api

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// LoggingMiddleware logs incoming requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.RequestURI, time.Since(start))
	})
}

// allowedOrigins returns the set of origins permitted for CORS.
// Reads from CORS_ORIGINS env var (comma-separated).
// Falls back to permissive "*" only in development.
var allowedOrigins = func() map[string]bool {
	raw := os.Getenv("CORS_ORIGINS")
	if raw == "" {
		// Sensible defaults for production
		return map[string]bool{
			"https://openchaos.dev":     true,
			"https://www.openchaos.dev": true,
			"https://feed.openchaos.dev": true,
		}
	}
	m := make(map[string]bool)
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			m[o] = true
		}
	}
	return m
}()

// CORSMiddleware adds CORS headers for cross-origin requests
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// In development, allow all origins
		if os.Getenv("ENV") == "development" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
		w.Header().Set("Access-Control-Max-Age", "300")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
