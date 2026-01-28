package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds application configuration
type Config struct {
	Port        string
	Env         string
	DatabaseURL string
	GitHubToken string
	GitHubRepo  string

	// Feed ingestion intervals
	GitHubPollInterval        time.Duration
	GitHubReactionsInterval   time.Duration
	GitHubDiscussionsInterval time.Duration
}

// Load reads configuration from environment variables.
// Returns an error if required variables are missing.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is required")
	}

	return &Config{
		Port:        getEnv("PORT", "8080"),
		Env:         getEnv("ENV", "development"),
		DatabaseURL: dbURL,
		GitHubToken: ghToken,
		GitHubRepo:  getEnv("GITHUB_REPO", "skridlevsky/openchaos"),

		GitHubPollInterval:        getDuration("GITHUB_POLL_INTERVAL", 60*time.Second),
		GitHubReactionsInterval:   getDuration("GITHUB_REACTIONS_INTERVAL", 5*time.Minute),
		GitHubDiscussionsInterval: getDuration("GITHUB_DISCUSSIONS_INTERVAL", 10*time.Minute),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
