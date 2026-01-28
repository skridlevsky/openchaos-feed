package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skridlevsky/openchaos-feed/internal/feed"
)

// FeedHandler handles feed-related requests
type FeedHandler struct {
	store    *feed.Store
	ingester *feed.Ingester
}

// NewFeedHandler creates a new feed handler
func NewFeedHandler(store *feed.Store, ingester *feed.Ingester) *FeedHandler {
	return &FeedHandler{
		store:    store,
		ingester: ingester,
	}
}

// FeedHealthResponse represents the feed health check response
type FeedHealthResponse struct {
	Status         string                  `json:"status"`
	LastEventAt    *string                 `json:"lastEventAt,omitempty"`
	EventsLastHour int                     `json:"eventsLastHour"`
	Ingesters      map[string]IngesterInfo `json:"ingesters"`
}

// IngesterInfo represents ingester status
type IngesterInfo struct {
	LastPoll string `json:"lastPoll"`
	Status   string `json:"status"`
}

// Health handles GET /api/feed/health
func (h *FeedHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	response := FeedHealthResponse{
		Status:    "healthy",
		Ingesters: make(map[string]IngesterInfo),
	}

	// Get last event time
	stats, err := h.store.GetStats(ctx)
	if err == nil && stats.LatestEventAt != nil {
		timeStr := stats.LatestEventAt.Format(time.RFC3339)
		response.LastEventAt = &timeStr
		response.EventsLastHour = stats.EventsLastHour
	}

	// Get ingester status if available
	if h.ingester != nil {
		status := h.ingester.Status()
		response.Ingesters["events_api"] = IngesterInfo{
			LastPoll: status.EventsLastPoll.Format(time.RFC3339),
			Status:   status.EventsStatus,
		}
		response.Ingesters["reactions"] = IngesterInfo{
			LastPoll: status.ReactionsLastPoll.Format(time.RFC3339),
			Status:   status.ReactionsStatus,
		}
		response.Ingesters["discussions"] = IngesterInfo{
			LastPoll: status.DiscussionsLastPoll.Format(time.RFC3339),
			Status:   status.DiscussionsStatus,
		}
	}

	respondJSON(w, http.StatusOK, response)
}

// ListResponse represents paginated feed list response
type ListResponse struct {
	Events     []*feed.Event `json:"events"`
	NextCursor *string       `json:"nextCursor,omitempty"`
	TotalCount int           `json:"totalCount"`
}

// List handles GET /api/feed
func (h *FeedHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "newest"
	}

	typeFilter := r.URL.Query().Get("type")
	prStr := r.URL.Query().Get("pr")
	userFilter := r.URL.Query().Get("user")
	sinceStr := r.URL.Query().Get("since")
	untilStr := r.URL.Query().Get("until")
	limitStr := r.URL.Query().Get("limit")
	cursor := r.URL.Query().Get("cursor")

	// Build filters
	filters := &feed.ListFilters{}

	if typeFilter != "" {
		for _, t := range strings.Split(typeFilter, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				filters.Types = append(filters.Types, feed.EventType(t))
			}
		}
	}

	if prStr != "" {
		if pr, err := strconv.Atoi(prStr); err == nil {
			filters.PRNumber = &pr
		}
	}

	if userFilter != "" {
		filters.GitHubUser = &userFilter
	}

	if sinceStr != "" {
		if since, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			filters.Since = &since
		}
	}

	if untilStr != "" {
		if until, err := time.Parse(time.RFC3339, untilStr); err == nil {
			filters.Until = &until
		}
	}

	// Parse limit
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Parse cursor
	var cursorPtr *string
	if cursor != "" {
		cursorPtr = &cursor
	}

	// Query events
	events, err := h.store.List(ctx, filters, sort, limit, cursorPtr)
	if err != nil {
		slog.Error("Failed to fetch events", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get total count
	totalCount, err := h.store.Count(ctx, filters)
	if err != nil {
		totalCount = 0 // Non-critical, continue
	}

	// Generate next cursor if we got a full page
	var nextCursor *string
	if len(events) == limit && len(events) > 0 {
		lastEvent := events[len(events)-1]
		nextCursor = &lastEvent.ID
	}

	response := ListResponse{
		Events:     events,
		NextCursor: nextCursor,
		TotalCount: totalCount,
	}

	respondJSON(w, http.StatusOK, response)
}

// StatsResponse represents feed statistics
type StatsResponse struct {
	TotalEvents    int                `json:"totalEvents"`
	TotalVotes     int                `json:"totalVotes"`
	TotalVoters    int                `json:"totalVoters"`
	LatestEventAt  *time.Time         `json:"latestEventAt,omitempty"`
	EventsByType   map[string]int     `json:"eventsByType"`
	EventsLastHour int                `json:"eventsLastHour"`
}

// Stats handles GET /api/feed/stats
func (h *FeedHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.store.GetStats(ctx)
	if err != nil {
		slog.Error("Failed to fetch stats", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := StatsResponse{
		TotalEvents:    stats.TotalEvents,
		TotalVotes:     stats.TotalVotes,
		TotalVoters:    stats.TotalVoters,
		LatestEventAt:  stats.LatestEventAt,
		EventsByType:   stats.EventsByType,
		EventsLastHour: stats.EventsLastHour,
	}

	respondJSON(w, http.StatusOK, response)
}

// GetEvent handles GET /api/feed/event/{id}
func (h *FeedHandler) GetEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if id == "" {
		http.Error(w, "Missing event ID", http.StatusBadRequest)
		return
	}

	event, err := h.store.GetByID(ctx, id)
	if err != nil {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}

	respondJSON(w, http.StatusOK, event)
}

// GetByPR handles GET /api/feed/pr/{number}
func (h *FeedHandler) GetByPR(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	numberStr := chi.URLParam(r, "number")

	number, err := strconv.Atoi(numberStr)
	if err != nil || number < 1 || number > 1000000 {
		http.Error(w, "Invalid PR number", http.StatusBadRequest)
		return
	}

	events, err := h.store.GetByPR(ctx, number)
	if err != nil {
		slog.Error("Failed to fetch PR events", "pr", number, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, events)
}

// GetByIssue handles GET /api/feed/issue/{number}
func (h *FeedHandler) GetByIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	numberStr := chi.URLParam(r, "number")

	number, err := strconv.Atoi(numberStr)
	if err != nil || number < 1 || number > 1000000 {
		http.Error(w, "Invalid issue number", http.StatusBadRequest)
		return
	}

	events, err := h.store.GetByIssue(ctx, number)
	if err != nil {
		slog.Error("Failed to fetch issue events", "issue", number, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, events)
}

// GetByUser handles GET /api/feed/user/{username}
func (h *FeedHandler) GetByUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := chi.URLParam(r, "username")

	if username == "" || len(username) > 39 {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}

	events, err := h.store.GetByUser(ctx, username)
	if err != nil {
		slog.Error("Failed to fetch user events", "user", username, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, events)
}

// GetVoters handles GET /api/feed/voters
// This is the CRITICAL endpoint for TU Delft Sybil research
func (h *FeedHandler) GetVoters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	voters, err := h.store.GetVoters(ctx)
	if err != nil {
		slog.Error("Failed to fetch voters", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, voters)
}

// GetVoter handles GET /api/feed/voters/{username}
func (h *FeedHandler) GetVoter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := chi.URLParam(r, "username")

	if username == "" || len(username) > 39 {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}

	voter, err := h.store.GetVoter(ctx, username)
	if err != nil {
		http.Error(w, "Voter not found", http.StatusNotFound)
		return
	}

	respondJSON(w, http.StatusOK, voter)
}

// PRVotesResponse represents vote breakdown for a PR
type PRVotesResponse struct {
	PRNumber  int                `json:"prNumber"`
	Upvotes   int                `json:"upvotes"`
	Downvotes int                `json:"downvotes"`
	Net       int                `json:"net"`
	Voters    []VoterVoteDetails `json:"voters"`
}

// VoterVoteDetails represents individual voter details for a PR
type VoterVoteDetails struct {
	GitHubUser string    `json:"githubUser"`
	Choice     int8      `json:"choice"` // +1 or -1
	VotedAt    time.Time `json:"votedAt"`
}

// GetPRVotes handles GET /api/feed/votes/pr/{number}
func (h *FeedHandler) GetPRVotes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	numberStr := chi.URLParam(r, "number")

	number, err := strconv.Atoi(numberStr)
	if err != nil || number < 1 || number > 1000000 {
		http.Error(w, "Invalid PR number", http.StatusBadRequest)
		return
	}

	// Get vote breakdown
	upvotes, downvotes, err := h.store.GetPRVotes(ctx, number)
	if err != nil {
		slog.Error("Failed to fetch PR votes", "pr", number, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get detailed voter list
	voteDetails, err := h.store.GetPRVoteDetails(ctx, number)
	if err != nil {
		slog.Error("Failed to fetch vote details", "pr", number, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	voters := make([]VoterVoteDetails, len(voteDetails))
	for i, detail := range voteDetails {
		voters[i] = VoterVoteDetails{
			GitHubUser: detail.GitHubUser,
			Choice:     detail.Choice,
			VotedAt:    detail.OccurredAt,
		}
	}

	response := PRVotesResponse{
		PRNumber:  number,
		Upvotes:   upvotes,
		Downvotes: downvotes,
		Net:       upvotes - downvotes,
		Voters:    voters,
	}

	respondJSON(w, http.StatusOK, response)
}

// Export handles GET /api/feed/export
// Bulk export for researchers — streams all events as NDJSON or CSV.
// Supports the same filters as List: type, pr, user, since, until, sort.
// Uses cursor pagination internally with 1000-event pages.
// Protected by: strict rate limit (2/min/IP), concurrency cap (3 global), 30s timeout.
func (h *FeedHandler) Export(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "ndjson"
	}
	if format != "ndjson" && format != "csv" {
		http.Error(w, "Invalid format (use ndjson or csv)", http.StatusBadRequest)
		return
	}

	// Parse filters (same as List)
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "oldest" // Default to chronological for research
	}
	typeFilter := r.URL.Query().Get("type")
	prStr := r.URL.Query().Get("pr")
	userFilter := r.URL.Query().Get("user")
	sinceStr := r.URL.Query().Get("since")
	untilStr := r.URL.Query().Get("until")

	filters := &feed.ListFilters{}
	if typeFilter != "" {
		for _, t := range strings.Split(typeFilter, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				filters.Types = append(filters.Types, feed.EventType(t))
			}
		}
	}
	if prStr != "" {
		if pr, err := strconv.Atoi(prStr); err == nil {
			filters.PRNumber = &pr
		}
	}
	if userFilter != "" {
		filters.GitHubUser = &userFilter
	}
	if sinceStr != "" {
		if since, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			filters.Since = &since
		}
	}
	if untilStr != "" {
		if until, err := time.Parse(time.RFC3339, untilStr); err == nil {
			filters.Until = &until
		}
	}

	// Set response headers
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=openchaos-feed-export.csv")
	} else {
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=openchaos-feed-export.ndjson")
	}
	w.WriteHeader(http.StatusOK)

	var csvWriter *csv.Writer
	if format == "csv" {
		csvWriter = csv.NewWriter(w)
		if err := csvWriter.Write([]string{
			"id", "type", "github_user", "github_user_id",
			"pr_number", "issue_number", "discussion_number",
			"choice", "reaction_type", "occurred_at", "ingested_at",
		}); err != nil {
			slog.Error("Export failed to write CSV header", "error", err)
			return
		}
	}

	encoder := json.NewEncoder(w)
	var cursor *string
	totalExported := 0
	maxExport := 100000 // Safety cap

	for totalExported < maxExport {
		// Check context timeout between pages
		if ctx.Err() != nil {
			slog.Info("Export terminated by timeout", "exported_so_far", totalExported)
			break
		}

		events, err := h.store.ExportList(ctx, filters, sort, 1000, cursor)
		if err != nil {
			slog.Error("Export query failed", "error", err, "exported_so_far", totalExported)
			break
		}

		if len(events) == 0 {
			break
		}

		writeErr := false
		for _, event := range events {
			if format == "csv" {
				if err := csvWriter.Write([]string{
					event.ID,
					string(event.Type),
					event.GitHubUser,
					strconv.FormatInt(event.GitHubUserID, 10),
					intPtrStr(event.PRNumber),
					intPtrStr(event.IssueNumber),
					intPtrStr(event.DiscussionNumber),
					int8PtrStr(event.Choice),
					strPtrStr(event.ReactionType),
					event.OccurredAt.Format(time.RFC3339),
					event.IngestedAt.Format(time.RFC3339),
				}); err != nil {
					slog.Info("Export write error (client likely disconnected)", "exported_so_far", totalExported, "error", err)
					writeErr = true
					break
				}
			} else {
				if err := encoder.Encode(event); err != nil {
					slog.Info("Export write error (client likely disconnected)", "exported_so_far", totalExported, "error", err)
					writeErr = true
					break
				}
			}
			totalExported++
		}
		if writeErr {
			break
		}

		if format == "csv" {
			csvWriter.Flush()
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Set cursor for next page
		lastID := events[len(events)-1].ID
		cursor = &lastID

		if len(events) < 1000 {
			break // Last page
		}
	}

	if format == "csv" {
		csvWriter.Flush()
	}
}

func intPtrStr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

func int8PtrStr(p *int8) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d", *p)
}

func strPtrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
