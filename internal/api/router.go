package api

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/skridlevsky/openchaos-feed/internal/feed"
)

// RouterConfig holds configuration for the router
type RouterConfig struct {
	Database interface{ Health(context.Context) error }
	FeedStore *feed.Store
	Ingester  *feed.Ingester
}

// RouterResult holds the router and resources that need cleanup
type RouterResult struct {
	Router       *chi.Mux
	RateLimiters *RateLimiters
}

// NewRouter creates and configures the HTTP router.
// Caller must call result.RateLimiters.Stop() on shutdown.
func NewRouter(cfg *RouterConfig) *RouterResult {
	r := chi.NewRouter()

	// Initialize rate limiters
	rateLimiters := NewRateLimiters()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(LoggingMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(CORSMiddleware)
	r.Use(rateLimiters.Global.Middleware)

	// Health endpoint
	if cfg.Database != nil {
		r.Get("/api/health", NewHealthHandler(cfg.Database))
	} else {
		r.Get("/api/health", HealthHandler)
	}

	// Feed API
	feedHandler := NewFeedHandler(cfg.FeedStore, cfg.Ingester)
	r.Route("/api/feed", func(r chi.Router) {
		r.Get("/health", feedHandler.Health)
		r.Get("/", feedHandler.List)
		r.Get("/stats", feedHandler.Stats)
		r.Get("/event/{id}", feedHandler.GetEvent)
		r.Get("/pr/{number}", feedHandler.GetByPR)
		r.Get("/issue/{number}", feedHandler.GetByIssue)
		r.Get("/user/{username}", feedHandler.GetByUser)
		r.Get("/voters", feedHandler.GetVoters)
		r.Get("/voters/{username}", feedHandler.GetVoter)
		r.Get("/votes/pr/{number}", feedHandler.GetPRVotes)

		// Export: strict rate limit (2/min/IP) + concurrency cap (3 global) + 30s timeout
		r.With(ExportGuardMiddleware(rateLimiters.Export)).
			Get("/export", feedHandler.Export)
	})

	return &RouterResult{
		Router:       r,
		RateLimiters: rateLimiters,
	}
}
