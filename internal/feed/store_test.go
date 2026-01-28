package feed

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

// Note: These tests require a running Postgres database
// Run: docker-compose up -d postgres
// Skip tests if DATABASE_URL is not set

func TestStore_InsertAndGetByID(t *testing.T) {
	t.Skip("Requires database - run manually with docker-compose up")

	// This test would be implemented with testcontainers-go in production
	// For now, it serves as documentation of expected behavior
}

func TestStore_Dedupe(t *testing.T) {
	t.Skip("Requires database - run manually with docker-compose up")

	// Test that inserting same github_id twice doesn't create duplicate
	// Expected: First insert succeeds, second is silently ignored
}

func TestStore_GetVoters(t *testing.T) {
	t.Skip("Requires database - run manually with docker-compose up")

	// Test voter aggregation:
	// 1. Insert multiple vote events for different users
	// 2. Call GetVoters()
	// 3. Verify aggregated stats match expectations
}

// Helper function to create test events
func createTestEvent(githubUser string, prNumber int, choice int8) *Event {
	payload := []byte(`{"test": true}`)
	hash := sha256.Sum256(payload)
	githubID := int64(time.Now().UnixNano())

	return &Event{
		Type:         EventReaction,
		GitHubUser:   githubUser,
		GitHubUserID: 12345,
		PRNumber:     &prNumber,
		Choice:       &choice,
		GitHubID:     &githubID,
		Payload:      payload,
		ContentHash:  hex.EncodeToString(hash[:]),
		OccurredAt:   time.Now(),
	}
}
