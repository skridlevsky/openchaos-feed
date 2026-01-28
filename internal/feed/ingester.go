package feed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skridlevsky/openchaos-feed/internal/github"
)

// GraphQLClient defines the interface for GraphQL operations
type GraphQLClient interface {
	FetchDiscussions(ctx context.Context, owner, repo string) ([]github.Discussion, error)
}

// Ingester coordinates polling of GitHub APIs for event ingestion
type Ingester struct {
	githubClient     *github.Client
	graphqlClient    GraphQLClient
	store            *Store
	owner            string
	repo             string
	eventsInterval   time.Duration
	reactionsInterval time.Duration
	discussionsInterval time.Duration

	// State tracking
	lastEventETag    string
	openPRs          map[int]bool // Track which PRs are open for prioritized polling
	reactionsCycle   int          // Counter for full-scan cadence (every 10th cycle polls all PRs)
	mu               sync.RWMutex

	// Status tracking for health endpoint
	eventsLastPoll      time.Time
	reactionsLastPoll   time.Time
	discussionsLastPoll time.Time
	eventsStatus        string
	reactionsStatus     string
	discussionsStatus   string
	statusMu            sync.RWMutex

	// Lifecycle
	stopCh           chan struct{}
	stopOnce         sync.Once
	wg               sync.WaitGroup
}

// NewIngester creates a new event ingester.
// Returns an error if ownerRepo is not in "owner/repo" format.
func NewIngester(
	githubClient *github.Client,
	graphqlClient GraphQLClient,
	store *Store,
	ownerRepo string,
	eventsInterval, reactionsInterval, discussionsInterval time.Duration,
) (*Ingester, error) {
	parts := strings.Split(ownerRepo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s (expected owner/repo)", ownerRepo)
	}

	return &Ingester{
		githubClient:     githubClient,
		graphqlClient:    graphqlClient,
		store:            store,
		owner:            parts[0],
		repo:             parts[1],
		eventsInterval:   eventsInterval,
		reactionsInterval: reactionsInterval,
		discussionsInterval: discussionsInterval,
		openPRs:          make(map[int]bool),
		stopCh:           make(chan struct{}),
	}, nil
}

// Run starts all polling loops
func (ing *Ingester) Run(ctx context.Context) {
	slog.Info("Ingester starting",
		"owner", ing.owner,
		"repo", ing.repo,
		"events_interval", ing.eventsInterval,
		"reactions_interval", ing.reactionsInterval,
		"discussions_interval", ing.discussionsInterval,
	)

	// Start Events API poller
	ing.wg.Add(1)
	go ing.pollEvents(ctx)

	// Start Reactions API poller (THE VOTES!)
	ing.wg.Add(1)
	go ing.pollReactions(ctx)

	// Start Discussions GraphQL poller
	ing.wg.Add(1)
	go ing.pollDiscussions(ctx)

	slog.Info("Ingester started - all pollers running")
}

// Stop gracefully shuts down the ingester. Safe to call multiple times.
func (ing *Ingester) Stop() {
	ing.stopOnce.Do(func() {
		slog.Info("Ingester stopping...")
		close(ing.stopCh)
		ing.wg.Wait()
		slog.Info("Ingester stopped")
	})
}

// pollEvents polls the GitHub Events API every N seconds
func (ing *Ingester) pollEvents(ctx context.Context) {
	defer ing.wg.Done()

	ticker := time.NewTicker(ing.eventsInterval)
	defer ticker.Stop()

	// Poll immediately on startup
	ing.fetchAndProcessEvents(ctx)

	for {
		select {
		case <-ticker.C:
			ing.fetchAndProcessEvents(ctx)
		case <-ing.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// IngesterStatus represents the status of all ingesters
type IngesterStatus struct {
	EventsLastPoll      time.Time
	EventsStatus        string
	ReactionsLastPoll   time.Time
	ReactionsStatus     string
	DiscussionsLastPoll time.Time
	DiscussionsStatus   string
}

// Status returns the current status of all ingesters
func (ing *Ingester) Status() *IngesterStatus {
	ing.statusMu.RLock()
	defer ing.statusMu.RUnlock()

	return &IngesterStatus{
		EventsLastPoll:      ing.eventsLastPoll,
		EventsStatus:        ing.eventsStatus,
		ReactionsLastPoll:   ing.reactionsLastPoll,
		ReactionsStatus:     ing.reactionsStatus,
		DiscussionsLastPoll: ing.discussionsLastPoll,
		DiscussionsStatus:   ing.discussionsStatus,
	}
}

// fetchAndProcessEvents fetches events from GitHub and processes them
func (ing *Ingester) fetchAndProcessEvents(ctx context.Context) {
	// Update status
	ing.statusMu.Lock()
	ing.eventsLastPoll = time.Now()
	ing.eventsStatus = "running"
	ing.statusMu.Unlock()

	etag := ing.lastEventETag
	events, headers, err := ing.githubClient.GetRepoEvents(ctx, ing.owner, ing.repo, &etag)
	if err != nil {
		slog.Error("Failed to fetch events", "error", err)
		ing.statusMu.Lock()
		ing.eventsStatus = "error: " + err.Error()
		ing.statusMu.Unlock()
		return
	}

	// Get rate limit info and backoff if needed
	rateLimit := github.GetRateLimitFromHeaders(headers)
	slog.Debug("Events API polled",
		"rate_limit_remaining", rateLimit.Remaining,
		"etag_cached", events == nil,
	)

	if rateLimit.Remaining < 10 && !rateLimit.Reset.IsZero() {
		sleepDur := time.Until(rateLimit.Reset)
		if sleepDur > 0 && sleepDur < 15*time.Minute {
			slog.Warn("GitHub rate limit low, backing off",
				"remaining", rateLimit.Remaining,
				"sleep", sleepDur.Round(time.Second),
			)
			time.Sleep(sleepDur)
		}
	}

	// If 304 Not Modified, no new events
	if events == nil {
		slog.Debug("Events API: no new events (ETag cache hit)")
		return
	}

	// Update ETag for next request
	newETag := headers.Get("ETag")
	if newETag != "" {
		ing.lastEventETag = newETag
	}

	// Process each event
	processedCount := 0
	dbErrors := 0
	for _, rawEvent := range events {
		// If we get multiple consecutive DB errors, stop processing this cycle
		// to avoid burning through events while the DB is down
		if dbErrors >= 3 {
			slog.Warn("Stopping event processing due to repeated DB errors",
				"db_errors", dbErrors,
				"events_remaining", len(events)-processedCount,
			)
			break
		}

		feedEvents, err := ing.parseGitHubEvent(ctx, &rawEvent)
		if err != nil {
			slog.Warn("Failed to parse event",
				"event_id", rawEvent.ID,
				"event_type", rawEvent.Type,
				"error", err,
			)
			continue
		}

		for _, feedEvent := range feedEvents {
			if err := ing.store.Insert(ctx, feedEvent); err != nil {
				slog.Error("Failed to insert event",
					"event_type", feedEvent.Type,
					"github_user", feedEvent.GitHubUser,
					"error", err,
				)
				dbErrors++
				continue
			}
			dbErrors = 0 // Reset on success
			processedCount++
		}
	}

	if processedCount > 0 {
		slog.Info("Events API processed",
			"new_events", processedCount,
			"rate_limit_remaining", rateLimit.Remaining,
		)
	}
}

// parseGitHubEvent parses a raw GitHub event into feed event(s)
func (ing *Ingester) parseGitHubEvent(ctx context.Context, raw *github.RawGitHubEvent) ([]*Event, error) {
	events := []*Event{}

	// Parse the Events API event ID for deduplication.
	// Every event from the GitHub Events API has a unique numeric string ID.
	// We use this as the default GitHubID for events that don't have their own
	// inner ID (e.g., push, star, branch create/delete, wiki, member events).
	var rawEventID *int64
	if raw.ID != "" {
		if parsed, err := strconv.ParseInt(raw.ID, 10, 64); err == nil {
			rawEventID = &parsed
		}
	}

	// Parse based on event type
	switch raw.Type {
	case "PullRequestEvent":
		var payload github.PullRequestEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse PullRequestEvent: %w", err)
		}

		var eventType EventType
		switch payload.Action {
		case "opened":
			eventType = EventPROpened
		case "closed":
			if payload.PullRequest.Merged {
				eventType = EventPRMerged
			} else {
				eventType = EventPRClosed
			}
		case "reopened":
			eventType = EventPRReopened
		case "edited":
			eventType = EventPREdited
		case "synchronize":
			eventType = EventPRSynchronized
		default:
			return nil, nil // Unknown action, skip
		}

		// Track open PRs for reactions polling
		if payload.Action == "opened" || payload.Action == "reopened" {
			ing.mu.Lock()
			ing.openPRs[payload.Number] = true
			ing.mu.Unlock()
		} else if payload.Action == "closed" {
			ing.mu.Lock()
			delete(ing.openPRs, payload.Number)
			ing.mu.Unlock()
		}

		githubID := int64(payload.PullRequest.ID)
		events = append(events, &Event{
			Type:         eventType,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,
			PRNumber:     &payload.Number,
			GitHubID:     &githubID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   raw.CreatedAt,
		})

	case "IssueCommentEvent":
		var payload github.IssueCommentEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse IssueCommentEvent: %w", err)
		}

		if payload.Action == "deleted" {
			commentID := int64(payload.Comment.ID)
			if err := ing.store.DeleteByCommentID(ctx, commentID); err != nil {
				slog.Debug("Failed to delete comment (may not exist)", "comment_id", commentID, "error", err)
			} else {
				slog.Info("Comment deleted", "comment_id", commentID)
			}
			return nil, nil
		}

		if payload.Action == "edited" {
			var changes struct {
				Body struct {
					From string `json:"from"`
				} `json:"body"`
			}
			var rawPayload map[string]json.RawMessage
			if err := json.Unmarshal(raw.Payload, &rawPayload); err == nil {
				if changesRaw, ok := rawPayload["changes"]; ok {
					json.Unmarshal(changesRaw, &changes)
				}
			}
			if changes.Body.From != "" {
				commentID := int64(payload.Comment.ID)
				if err := ing.store.UpdateCommentEdit(ctx, commentID, raw.Payload, changes.Body.From, raw.CreatedAt); err != nil {
					slog.Warn("Failed to update comment edit",
						"comment_id", commentID,
						"error", err,
					)
				} else {
					slog.Info("Comment edit recorded",
						"comment_id", commentID,
						"github_user", payload.Comment.User.Login,
					)
				}
			}
			return nil, nil
		}

		if payload.Action != "created" {
			return nil, nil // Only track new comments and edits
		}

		githubID := int64(payload.Comment.ID)
		commentID := int64(payload.Comment.ID)
		event := &Event{
			Type:         EventIssueComment,
			GitHubUser:   payload.Comment.User.Login,
			GitHubUserID: payload.Comment.User.ID,

			CommentID:    &commentID,
			GitHubID:     &githubID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   payload.Comment.CreatedAt,
		}

		// Determine if comment is on PR or issue
		if payload.Issue.PullRequest != nil {
			event.PRNumber = &payload.Issue.Number
		} else {
			event.IssueNumber = &payload.Issue.Number
		}

		events = append(events, event)

	case "PushEvent":
		var payload github.PushEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse PushEvent: %w", err)
		}

		events = append(events, &Event{
			Type:         EventPush,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,
			GitHubID:     rawEventID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   raw.CreatedAt,
		})

	case "WatchEvent":
		events = append(events, &Event{
			Type:         EventStar,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,

			GitHubID:     rawEventID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   raw.CreatedAt,
		})

	case "ForkEvent":
		var payload github.ForkEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse ForkEvent: %w", err)
		}

		githubID := int64(payload.Forkee.ID)
		events = append(events, &Event{
			Type:         EventFork,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,

			GitHubID:     &githubID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   payload.Forkee.CreatedAt,
		})

	case "IssuesEvent":
		var payload github.IssuesEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse IssuesEvent: %w", err)
		}

		var eventType EventType
		switch payload.Action {
		case "opened":
			eventType = EventIssueOpened
		case "closed":
			eventType = EventIssueClosed
		case "reopened":
			eventType = EventIssueReopened
		case "edited":
			eventType = EventIssueEdited
		default:
			return nil, nil
		}

		githubID := int64(payload.Issue.ID)
		events = append(events, &Event{
			Type:         eventType,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,

			IssueNumber:  &payload.Issue.Number,
			GitHubID:     &githubID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   raw.CreatedAt,
		})

	case "PullRequestReviewEvent":
		var payload github.PullRequestReviewEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse PullRequestReviewEvent: %w", err)
		}

		if payload.Action != "submitted" {
			return nil, nil
		}

		githubID := int64(payload.Review.ID)
		events = append(events, &Event{
			Type:         EventReviewSubmitted,
			GitHubUser:   payload.Review.User.Login,
			GitHubUserID: payload.Review.User.ID,

			PRNumber:     &payload.PullRequest.Number,
			GitHubID:     &githubID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   payload.Review.SubmittedAt,
		})

	case "PullRequestReviewCommentEvent":
		var payload github.PullRequestReviewCommentEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse PullRequestReviewCommentEvent: %w", err)
		}

		if payload.Action == "deleted" {
			commentID := int64(payload.Comment.ID)
			if err := ing.store.DeleteByCommentID(ctx, commentID); err != nil {
				slog.Debug("Failed to delete review comment (may not exist)", "comment_id", commentID, "error", err)
			} else {
				slog.Info("Review comment deleted", "comment_id", commentID)
			}
			return nil, nil
		}

		if payload.Action == "edited" {
			var changes struct {
				Body struct {
					From string `json:"from"`
				} `json:"body"`
			}
			var rawPayload map[string]json.RawMessage
			if err := json.Unmarshal(raw.Payload, &rawPayload); err == nil {
				if changesRaw, ok := rawPayload["changes"]; ok {
					json.Unmarshal(changesRaw, &changes)
				}
			}
			if changes.Body.From != "" {
				commentID := int64(payload.Comment.ID)
				if err := ing.store.UpdateCommentEdit(ctx, commentID, raw.Payload, changes.Body.From, raw.CreatedAt); err != nil {
					slog.Warn("Failed to update review comment edit",
						"comment_id", commentID,
						"error", err,
					)
				} else {
					slog.Info("Review comment edit recorded",
						"comment_id", commentID,
						"github_user", payload.Comment.User.Login,
					)
				}
			}
			return nil, nil
		}

		if payload.Action != "created" {
			return nil, nil
		}

		githubID := int64(payload.Comment.ID)
		commentID := int64(payload.Comment.ID)
		events = append(events, &Event{
			Type:         EventReviewComment,
			GitHubUser:   payload.Comment.User.Login,
			GitHubUserID: payload.Comment.User.ID,

			PRNumber:     &payload.PullRequest.Number,
			CommentID:    &commentID,
			GitHubID:     &githubID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   payload.Comment.CreatedAt,
		})

	case "CreateEvent":
		var payload github.CreateEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse CreateEvent: %w", err)
		}

		var eventType EventType
		switch payload.RefType {
		case "branch":
			eventType = EventBranchCreated
		case "tag":
			eventType = EventTagCreated
		default:
			return nil, nil
		}

		events = append(events, &Event{
			Type:         eventType,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,

			GitHubID:     rawEventID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   raw.CreatedAt,
		})

	case "DeleteEvent":
		var payload github.DeleteEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse DeleteEvent: %w", err)
		}

		var eventType EventType
		switch payload.RefType {
		case "branch":
			eventType = EventBranchDeleted
		case "tag":
			eventType = EventTagDeleted
		default:
			return nil, nil
		}

		events = append(events, &Event{
			Type:         eventType,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,

			GitHubID:     rawEventID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   raw.CreatedAt,
		})

	case "ReleaseEvent":
		var payload github.ReleaseEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse ReleaseEvent: %w", err)
		}

		if payload.Action != "published" {
			return nil, nil
		}

		githubID := int64(payload.Release.ID)
		events = append(events, &Event{
			Type:         EventRelease,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,

			GitHubID:     &githubID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   payload.Release.PublishedAt,
		})

	case "GollumEvent":
		events = append(events, &Event{
			Type:         EventWikiEdit,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,

			GitHubID:     rawEventID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   raw.CreatedAt,
		})

	case "MemberEvent":
		var payload github.MemberEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse MemberEvent: %w", err)
		}

		if payload.Action != "added" {
			return nil, nil
		}

		events = append(events, &Event{
			Type:         EventCollaboratorAdded,
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,

			GitHubID:     rawEventID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   raw.CreatedAt,
		})

	case "CommitCommentEvent":
		var payload github.CommitCommentEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse CommitCommentEvent: %w", err)
		}

		githubID := int64(payload.Comment.ID)
		commentID := int64(payload.Comment.ID)
		events = append(events, &Event{
			Type:         EventCommitComment,
			GitHubUser:   payload.Comment.User.Login,
			GitHubUserID: payload.Comment.User.ID,

			CommentID:    &commentID,
			GitHubID:     &githubID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   payload.Comment.CreatedAt,
		})

	case "DiscussionEvent":
		var payload github.DiscussionEventPayload
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse DiscussionEvent: %w", err)
		}

		if payload.Action != "created" {
			return nil, nil
		}

		githubID := int64(payload.Discussion.ID)
		discussionNumber := payload.Discussion.Number
		events = append(events, &Event{
			Type:             EventDiscussionCreated,
			GitHubUser:       payload.Discussion.User.Login,
			GitHubUserID:     payload.Discussion.User.ID,

			DiscussionNumber: &discussionNumber,
			GitHubID:         &githubID,
			Payload:          raw.Payload,
			ContentHash:      computeContentHash(raw.Payload),
			OccurredAt:       payload.Discussion.CreatedAt,
		})

	case "PublicEvent":
		// Repository made public - minimal payload
		events = append(events, &Event{
			Type:         EventType("repo_public"),
			GitHubUser:   raw.Actor.Login,
			GitHubUserID: raw.Actor.ID,

			GitHubID:     rawEventID,
			Payload:      raw.Payload,
			ContentHash:  computeContentHash(raw.Payload),
			OccurredAt:   raw.CreatedAt,
		})

	default:
		// Unknown event type - log but don't fail
		slog.Debug("Unknown event type", "type", raw.Type)
		return nil, nil
	}

	return events, nil
}

// computeContentHash computes SHA256 hash of payload for deduplication
func computeContentHash(payload []byte) string {
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

// pollReactions polls the Reactions API for votes (THE MOST CRITICAL INGESTER!)
func (ing *Ingester) pollReactions(ctx context.Context) {
	defer ing.wg.Done()

	ticker := time.NewTicker(ing.reactionsInterval)
	defer ticker.Stop()

	// Poll immediately on startup
	ing.fetchAndProcessReactions(ctx)

	for {
		select {
		case <-ticker.C:
			ing.fetchAndProcessReactions(ctx)
		case <-ing.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// fetchAndProcessReactions fetches reactions for all PRs (THE VOTES!)
// Open PRs are polled every cycle. All PRs (including closed/merged) are
// polled every 10th cycle to capture late votes without burning rate limit.
func (ing *Ingester) fetchAndProcessReactions(ctx context.Context) {
	// Update status
	ing.statusMu.Lock()
	ing.reactionsLastPoll = time.Now()
	ing.reactionsStatus = "running"
	ing.statusMu.Unlock()

	ing.mu.Lock()
	ing.reactionsCycle++
	cycle := ing.reactionsCycle
	ing.mu.Unlock()

	// Every 10th cycle, poll ALL PRs (open + closed) to catch late votes
	pollAll := cycle%10 == 0

	var prNumbers []int

	if pollAll {
		allPRs, err := ing.githubClient.GetAllPRs(ctx, ing.owner, ing.repo)
		if err != nil {
			slog.Error("Failed to fetch all PRs for reactions", "error", err)
			ing.statusMu.Lock()
			ing.reactionsStatus = "error: " + err.Error()
			ing.statusMu.Unlock()
			return
		}
		for _, pr := range allPRs {
			prNumbers = append(prNumbers, pr.Number)
		}
		slog.Info("Reactions: full PR scan", "total_prs", len(prNumbers))
	} else {
		prs, err := ing.githubClient.GetOpenPRs(ctx, ing.owner, ing.repo)
		if err != nil {
			slog.Error("Failed to fetch open PRs for reactions", "error", err)
			ing.statusMu.Lock()
			ing.reactionsStatus = "error: " + err.Error()
			ing.statusMu.Unlock()
			return
		}
		for _, pr := range prs {
			prNumbers = append(prNumbers, pr.Number)
		}
	}

	totalReactions := 0
	dbErrors := 0
	for _, prNum := range prNumbers {
		if dbErrors >= 3 {
			slog.Warn("Stopping reaction processing due to repeated DB errors",
				"db_errors", dbErrors,
				"prs_remaining", len(prNumbers),
			)
			break
		}

		reactions, err := ing.githubClient.GetIssueReactions(ctx, ing.owner, ing.repo, prNum)
		if err != nil {
			slog.Error("Failed to fetch reactions for PR",
				"pr_number", prNum,
				"error", err,
			)
			continue
		}

		for _, reaction := range reactions {
			// Determine choice for votes (+1/-1)
			var choice *int8
			if reaction.Content == "+1" {
				c := int8(1)
				choice = &c
			} else if reaction.Content == "-1" {
				c := int8(-1)
				choice = &c
			}

			// Create reaction payload for storage
			reactionPayload, _ := json.Marshal(map[string]interface{}{
				"id":         reaction.ID,
				"content":    reaction.Content,
				"user":       reaction.User,
				"created_at": reaction.CreatedAt,
				"pr_number":  prNum,
			})

			githubID := reaction.ID
			prNumber := prNum
			reactionType := reaction.Content
			event := &Event{
				Type:         EventReaction,
				GitHubUser:   reaction.User.Login,
				GitHubUserID: reaction.User.ID,

				PRNumber:     &prNumber,
				Choice:       choice,
				ReactionType: &reactionType,
				GitHubID:     &githubID,
				Payload:      reactionPayload,
				ContentHash:  computeContentHash(reactionPayload),
				OccurredAt:   reaction.CreatedAt,
			}

			if err := ing.store.Insert(ctx, event); err != nil {
				slog.Error("Failed to insert reaction",
					"pr_number", prNum,
					"reaction_id", reaction.ID,
					"error", err,
				)
				dbErrors++
				continue
			}
			dbErrors = 0
			totalReactions++
		}
	}

	slog.Info("Reactions API processed",
		"prs_checked", len(prNumbers),
		"reactions_processed", totalReactions,
		"full_scan", pollAll,
	)
}

// pollDiscussions polls the GraphQL Discussions API
func (ing *Ingester) pollDiscussions(ctx context.Context) {
	defer ing.wg.Done()

	ticker := time.NewTicker(ing.discussionsInterval)
	defer ticker.Stop()

	// Poll immediately on startup
	ing.fetchAndProcessDiscussions(ctx)

	for {
		select {
		case <-ticker.C:
			ing.fetchAndProcessDiscussions(ctx)
		case <-ing.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// fetchAndProcessDiscussions fetches discussions via GraphQL
func (ing *Ingester) fetchAndProcessDiscussions(ctx context.Context) {
	// Update status
	ing.statusMu.Lock()
	ing.discussionsLastPoll = time.Now()
	ing.discussionsStatus = "running"
	ing.statusMu.Unlock()

	if ing.graphqlClient == nil {
		slog.Debug("GraphQL client not configured, skipping discussions poll")
		ing.statusMu.Lock()
		ing.discussionsStatus = "disabled"
		ing.statusMu.Unlock()
		return
	}

	discussions, err := ing.graphqlClient.FetchDiscussions(ctx, ing.owner, ing.repo)
	if err != nil {
		slog.Error("Failed to fetch discussions", "error", err)
		ing.statusMu.Lock()
		ing.discussionsStatus = "error: " + err.Error()
		ing.statusMu.Unlock()
		return
	}

	totalEvents := 0
	for _, discussion := range discussions {
		// Discussion creation event
		discussionPayload, _ := json.Marshal(discussion)

		discussionNumber := discussion.Number
		discussionID := int64(discussion.Number) // Use number as ID for deduping
		event := &Event{
			Type:             EventDiscussionCreated,
			GitHubUser:       discussion.Author.Login,
			GitHubUserID:     0, // GraphQL doesn't return user ID easily

			DiscussionNumber: &discussionNumber,
			GitHubID:         &discussionID,
			Payload:          discussionPayload,
			ContentHash:      computeContentHash(discussionPayload),
			OccurredAt:       discussion.CreatedAt,
		}

		if err := ing.store.Insert(ctx, event); err != nil {
			slog.Error("Failed to insert discussion event",
				"discussion_number", discussion.Number,
				"error", err,
			)
		} else {
			totalEvents++
		}

		// Discussion comments
		for _, comment := range discussion.Comments {
			commentPayload, _ := json.Marshal(comment)

			commentID := int64(comment.Number) // Use comment number as ID
			commentEvent := &Event{
				Type:             EventDiscussionComment,
				GitHubUser:       comment.Author.Login,
				GitHubUserID:     0,
				DiscussionNumber: &discussionNumber,
				CommentID:        &commentID,
				GitHubID:         &commentID,
				Payload:          commentPayload,
				ContentHash:      computeContentHash(commentPayload),
				OccurredAt:       comment.CreatedAt,
			}

			if err := ing.store.Insert(ctx, commentEvent); err != nil {
				slog.Error("Failed to insert discussion comment",
					"discussion_number", discussion.Number,
					"comment_id", comment.Number,
					"error", err,
				)
			} else {
				totalEvents++
			}
		}

		// Discussion reactions
		for _, reaction := range discussion.Reactions {
			reactionPayload, _ := json.Marshal(reaction)

			var choice *int8
			if reaction.Content == "+1" {
				c := int8(1)
				choice = &c
			} else if reaction.Content == "-1" {
				c := int8(-1)
				choice = &c
			}

			reactionID := int64(reaction.Number) // Use reaction number as ID
			reactionType := reaction.Content
			reactionEvent := &Event{
				Type:             EventReaction,
				GitHubUser:       reaction.User.Login,
				GitHubUserID:     0,
				DiscussionNumber: &discussionNumber,
				Choice:           choice,
				ReactionType:     &reactionType,
				GitHubID:         &reactionID,
				Payload:          reactionPayload,
				ContentHash:      computeContentHash(reactionPayload),
				OccurredAt:       reaction.CreatedAt,
			}

			if err := ing.store.Insert(ctx, reactionEvent); err != nil {
				slog.Error("Failed to insert discussion reaction",
					"discussion_number", discussion.Number,
					"reaction_id", reaction.Number,
					"error", err,
				)
			} else {
				totalEvents++
			}
		}
	}

	slog.Info("Discussions GraphQL processed",
		"discussions_fetched", len(discussions),
		"total_events", totalEvents,
	)
}
