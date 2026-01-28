package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/skridlevsky/openchaos-feed/internal/api"
	"github.com/skridlevsky/openchaos-feed/internal/config"
	"github.com/skridlevsky/openchaos-feed/internal/db"
	"github.com/skridlevsky/openchaos-feed/internal/feed"
	"github.com/skridlevsky/openchaos-feed/internal/github"
)

func main() {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Create context for services
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database connection
	database, err := db.NewPostgres(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	// NOTE: database.Close() called explicitly in shutdown sequence below — no defer

	// Run migrations
	if err := db.RunMigrations(ctx, database.Pool()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize feed store
	feedStore := feed.NewStore(database.Pool())
	log.Println("Feed store initialized")

	// Initialize feed ingester
	prCache := github.NewPRCache(5 * time.Minute)
	githubClient := github.NewClient(cfg.GitHubToken, prCache)
	graphqlClient := github.NewGraphQLClient(cfg.GitHubToken)

	ingester, err := feed.NewIngester(
		githubClient,
		graphqlClient,
		feedStore,
		cfg.GitHubRepo,
		cfg.GitHubPollInterval,
		cfg.GitHubReactionsInterval,
		cfg.GitHubDiscussionsInterval,
	)
	if err != nil {
		log.Fatalf("Failed to create ingester: %v", err)
	}
	ingester.Run(ctx)
	log.Println("Feed ingester started")

	// Create router
	routerResult := api.NewRouter(&api.RouterConfig{
		Database:  database,
		FeedStore: feedStore,
		Ingester:  ingester,
	})

	// Create server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      routerResult.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 35 * time.Second, // Must exceed Export handler's 30s context timeout
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting server on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Stop feed ingester
	log.Println("Stopping feed ingester...")
	ingester.Stop()

	// Stop rate limiter cleanup goroutines
	log.Println("Stopping rate limiters...")
	routerResult.RateLimiters.Stop()

	// Cancel context to stop all services
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	// Close database connection
	log.Println("Closing database connection...")
	database.Close()

	log.Println("Server exited")
}
