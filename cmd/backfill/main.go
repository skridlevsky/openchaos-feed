package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
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

	// Extract owner and repo
	owner, repo := parseRepo(cfg.GitHubRepo)

	// Create context
	ctx := context.Background()

	// Connect to database
	log.Println("Connecting to database...")
	database, err := db.NewPostgres(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Run migrations
	log.Println("Running migrations...")
	if err := db.RunMigrations(ctx, database.Pool()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize feed store
	store := feed.NewStore(database.Pool())

	// Initialize GitHub client (no cache needed for backfill)
	githubClient := github.NewClient(cfg.GitHubToken, nil)

	// Initialize GraphQL client for discussions
	graphqlClient := github.NewGraphQLClient(cfg.GitHubToken)

	log.Println("Starting historical backfill...")
	log.Printf("Repository: %s/%s\n", owner, repo)

	// Step 1: Fetch all PRs
	log.Println("Step 1/9: Fetching all PRs...")
	prs, err := githubClient.GetAllPRs(ctx, owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch PRs: %v", err)
	}
	log.Printf("Found %d PRs\n", len(prs))

	// Delete old PR events first (they have flat payload shape, not Events API shape)
	deleted, err := store.DeleteByTypes(ctx, []feed.EventType{
		feed.EventPROpened, feed.EventPRClosed, feed.EventPRMerged, feed.EventPRReopened,
	})
	if err != nil {
		log.Fatalf("Failed to delete old PR events: %v", err)
	}
	if deleted > 0 {
		log.Printf("  Deleted %d old PR events (will re-insert with correct payload shape)\n", deleted)
	}

	// Insert PR events with Events API-shaped payloads
	for i, pr := range prs {
		// Determine event type and action based on state
		var eventType feed.EventType
		var action string
		if pr.State == "closed" {
			action = "closed"
			if pr.Merged {
				eventType = feed.EventPRMerged
			} else {
				eventType = feed.EventPRClosed
			}
		} else {
			action = "opened"
			eventType = feed.EventPROpened
		}

		prNumber := pr.Number
		githubID := pr.ID // Use real GitHub API ID (avoids collision with star user IDs)

		// Build Events API-shaped payload: {action, number, pull_request: {...}}
		payload, _ := json.Marshal(map[string]interface{}{
			"action":       action,
			"number":       pr.Number,
			"pull_request": pr,
		})

		event := &feed.Event{
			Type:         eventType,
			GitHubUser:   pr.User.Login,
			GitHubUserID: pr.User.ID,
			PRNumber:     &prNumber,
			GitHubID:     &githubID,
			Payload:      payload,
			ContentHash:  computeContentHash(payload),
			OccurredAt:   parseTime(pr.CreatedAt),
		}

		if err := store.Insert(ctx, event); err != nil {
			slog.Warn("Failed to insert PR event", "pr", pr.Number, "error", err)
		}

		if (i+1)%10 == 0 || i+1 == len(prs) {
			log.Printf("  Progress: %d/%d PRs processed\n", i+1, len(prs))
		}

		// Check rate limit every 50 PRs
		if (i+1)%50 == 0 {
			checkRateLimit(ctx, githubClient)
		}
	}

	// Step 2: Fetch all issues
	log.Println("Step 2/9: Fetching all issues...")
	issues, err := githubClient.GetAllIssues(ctx, owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch issues: %v", err)
	}
	log.Printf("Found %d issues\n", len(issues))

	// Delete old issue events first (they have flat payload shape, not Events API shape)
	deletedIssues, err := store.DeleteByTypes(ctx, []feed.EventType{
		feed.EventIssueOpened, feed.EventIssueClosed, feed.EventIssueReopened,
	})
	if err != nil {
		log.Fatalf("Failed to delete old issue events: %v", err)
	}
	if deletedIssues > 0 {
		log.Printf("  Deleted %d old issue events (will re-insert with correct payload shape)\n", deletedIssues)
	}

	// Insert issue events with Events API-shaped payloads
	for i, issue := range issues {
		var eventType feed.EventType
		var action string
		if issue.State == "closed" {
			eventType = feed.EventIssueClosed
			action = "closed"
		} else {
			eventType = feed.EventIssueOpened
			action = "opened"
		}

		issueNumber := issue.Number
		githubID := issue.ID // Use real GitHub API ID

		// Build Events API-shaped payload: {action, issue: {...}}
		payload, _ := json.Marshal(map[string]interface{}{
			"action": action,
			"issue":  issue,
		})

		event := &feed.Event{
			Type:         eventType,
			GitHubUser:   issue.User.Login,
			GitHubUserID: issue.User.ID,
			IssueNumber:  &issueNumber,
			GitHubID:     &githubID,
			Payload:      payload,
			ContentHash:  computeContentHash(payload),
			OccurredAt:   issue.CreatedAt,
		}

		if err := store.Insert(ctx, event); err != nil {
			slog.Warn("Failed to insert issue event", "issue", issue.Number, "error", err)
		}

		if (i+1)%10 == 0 || i+1 == len(issues) {
			log.Printf("  Progress: %d/%d issues processed\n", i+1, len(issues))
		}
	}

	// Build lookup maps for resolving comment parents
	prByNumber := make(map[int]*github.GitHubPR, len(prs))
	for _, pr := range prs {
		prByNumber[pr.Number] = pr
	}
	issueByNumber := make(map[int]*github.GitHubIssue, len(issues))
	for i := range issues {
		issueByNumber[issues[i].Number] = &issues[i]
	}

	// Step 3: PR Reactions (THE VOTES!)
	log.Println("Step 3/9: Fetching PR reactions (THE VOTES!)...")
	totalReactions := 0
	for i, pr := range prs {
		reactions, err := githubClient.GetIssueReactions(ctx, owner, repo, pr.Number)
		if err != nil {
			slog.Warn("Failed to fetch reactions for PR", "pr", pr.Number, "error", err)
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

			reactionPayload, _ := json.Marshal(map[string]interface{}{
				"id":         reaction.ID,
				"content":    reaction.Content,
				"user":       reaction.User,
				"created_at": reaction.CreatedAt,
				"pr_number":  pr.Number,
			})

			githubID := reaction.ID
			prNumber := pr.Number
			reactionType := reaction.Content
			event := &feed.Event{
				Type:         feed.EventReaction,
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

			if err := store.Insert(ctx, event); err != nil {
				slog.Warn("Failed to insert reaction", "pr", pr.Number, "error", err)
			} else {
				totalReactions++
			}
		}

		if (i+1)%10 == 0 || i+1 == len(prs) {
			log.Printf("  Progress: %d/%d PRs processed, %d reactions captured\n", i+1, len(prs), totalReactions)
		}

		// Check rate limit every 20 PRs
		if (i+1)%20 == 0 {
			checkRateLimit(ctx, githubClient)
		}
	}
	log.Printf("Total PR reactions captured: %d\n", totalReactions)

	// Step 4: Issue Reactions
	log.Println("Step 4/9: Fetching issue reactions...")
	issueReactions := 0
	for i, issue := range issues {
		reactions, err := githubClient.GetIssueReactions(ctx, owner, repo, issue.Number)
		if err != nil {
			slog.Warn("Failed to fetch reactions for issue", "issue", issue.Number, "error", err)
			continue
		}

		for _, reaction := range reactions {
			var choice *int8
			if reaction.Content == "+1" {
				c := int8(1)
				choice = &c
			} else if reaction.Content == "-1" {
				c := int8(-1)
				choice = &c
			}

			reactionPayload, _ := json.Marshal(map[string]interface{}{
				"id":           reaction.ID,
				"content":      reaction.Content,
				"user":         reaction.User,
				"created_at":   reaction.CreatedAt,
				"issue_number": issue.Number,
			})

			githubID := reaction.ID
			issueNumber := issue.Number
			reactionType := reaction.Content
			event := &feed.Event{
				Type:         feed.EventReaction,
				GitHubUser:   reaction.User.Login,
				GitHubUserID: reaction.User.ID,
				IssueNumber:  &issueNumber,
				Choice:       choice,
				ReactionType: &reactionType,
				GitHubID:     &githubID,
				Payload:      reactionPayload,
				ContentHash:  computeContentHash(reactionPayload),
				OccurredAt:   reaction.CreatedAt,
			}

			if err := store.Insert(ctx, event); err != nil {
				slog.Warn("Failed to insert issue reaction", "issue", issue.Number, "error", err)
			} else {
				issueReactions++
			}
		}

		if (i+1)%10 == 0 || i+1 == len(issues) {
			log.Printf("  Progress: %d/%d issues processed, %d reactions captured\n", i+1, len(issues), issueReactions)
		}
	}
	log.Printf("Total issue reactions captured: %d\n", issueReactions)

	// Step 5: Fetch all comments
	log.Println("Step 5/9: Fetching all comments...")

	// Delete old comment events first (they have flat payload shape, not Events API shape)
	deleted, err = store.DeleteByType(ctx, feed.EventIssueComment)
	if err != nil {
		log.Fatalf("Failed to delete old comment events: %v", err)
	}
	if deleted > 0 {
		log.Printf("  Deleted %d old comment events (will re-insert with correct payload shape)\n", deleted)
	}

	comments, err := githubClient.GetAllComments(ctx, owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch comments: %v", err)
	}
	log.Printf("Found %d comments\n", len(comments))

	// Insert comment events with Events API-shaped payloads
	for i, comment := range comments {
		commentID := comment.ID
		githubID := comment.ID

		// Resolve parent issue/PR from the comment's issue_url
		parentNumber := parseIssueNumber(comment.IssueURL)
		var prNumber, issueNumber *int
		var parentTitle string

		if pr, ok := prByNumber[parentNumber]; ok {
			prNumber = &parentNumber
			parentTitle = pr.Title
		} else if issue, ok := issueByNumber[parentNumber]; ok {
			issueNumber = &parentNumber
			parentTitle = issue.Title
		}

		// Build Events API-shaped payload: {action, issue: {number, title, ...}, comment: {id, body, ...}}
		payload, _ := json.Marshal(map[string]interface{}{
			"action": "created",
			"issue": map[string]interface{}{
				"number": parentNumber,
				"title":  parentTitle,
			},
			"comment": map[string]interface{}{
				"id":         comment.ID,
				"body":       comment.Body,
				"user":       comment.User,
				"created_at": comment.CreatedAt,
				"updated_at": comment.UpdatedAt,
			},
		})

		event := &feed.Event{
			Type:         feed.EventIssueComment,
			GitHubUser:   comment.User.Login,
			GitHubUserID: comment.User.ID,
			PRNumber:     prNumber,
			IssueNumber:  issueNumber,
			CommentID:    &commentID,
			GitHubID:     &githubID,
			Payload:      payload,
			ContentHash:  computeContentHash(payload),
			OccurredAt:   comment.CreatedAt,
		}

		if err := store.Insert(ctx, event); err != nil {
			slog.Warn("Failed to insert comment", "comment_id", comment.ID, "error", err)
		}

		if (i+1)%50 == 0 || i+1 == len(comments) {
			log.Printf("  Progress: %d/%d comments processed\n", i+1, len(comments))
		}
	}

	// Step 6: Comment Reactions
	log.Println("Step 6/9: Fetching comment reactions...")
	commentReactions := 0
	for i, comment := range comments {
		reactions, err := githubClient.GetCommentReactions(ctx, owner, repo, comment.ID)
		if err != nil {
			slog.Warn("Failed to fetch reactions for comment", "comment_id", comment.ID, "error", err)
			continue
		}

		for _, reaction := range reactions {
			var choice *int8
			if reaction.Content == "+1" {
				c := int8(1)
				choice = &c
			} else if reaction.Content == "-1" {
				c := int8(-1)
				choice = &c
			}

			reactionPayload, _ := json.Marshal(map[string]interface{}{
				"id":         reaction.ID,
				"content":    reaction.Content,
				"user":       reaction.User,
				"created_at": reaction.CreatedAt,
				"comment_id": comment.ID,
			})

			githubID := reaction.ID
			commentID := comment.ID
			reactionType := reaction.Content
			event := &feed.Event{
				Type:         feed.EventReaction,
				GitHubUser:   reaction.User.Login,
				GitHubUserID: reaction.User.ID,
				CommentID:    &commentID,
				Choice:       choice,
				ReactionType: &reactionType,
				GitHubID:     &githubID,
				Payload:      reactionPayload,
				ContentHash:  computeContentHash(reactionPayload),
				OccurredAt:   reaction.CreatedAt,
			}

			if err := store.Insert(ctx, event); err != nil {
				slog.Warn("Failed to insert comment reaction", "comment_id", comment.ID, "error", err)
			} else {
				commentReactions++
			}
		}

		if (i+1)%50 == 0 || i+1 == len(comments) {
			log.Printf("  Progress: %d/%d comments processed, %d reactions captured\n", i+1, len(comments), commentReactions)
		}

		// Check rate limit every 50 comments
		if (i+1)%50 == 0 {
			checkRateLimit(ctx, githubClient)
		}
	}
	log.Printf("Total comment reactions captured: %d\n", commentReactions)

	// Step 7: Stargazers
	log.Println("Step 7/9: Fetching stargazers...")
	stargazers, err := githubClient.GetStargazersWithTimestamps(ctx, owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch stargazers: %v", err)
	}
	log.Printf("Found %d stargazers\n", len(stargazers))

	for i, stargazer := range stargazers {
		githubID := stargazer.User.ID
		payload, _ := json.Marshal(stargazer)

		event := &feed.Event{
			Type:         feed.EventStar,
			GitHubUser:   stargazer.User.Login,
			GitHubUserID: stargazer.User.ID,
			GitHubID:     &githubID,
			Payload:      payload,
			ContentHash:  computeContentHash(payload),
			OccurredAt:   stargazer.StarredAt,
		}

		if err := store.Insert(ctx, event); err != nil {
			slog.Warn("Failed to insert stargazer", "user", stargazer.User.Login, "error", err)
		}

		if (i+1)%50 == 0 || i+1 == len(stargazers) {
			log.Printf("  Progress: %d/%d stargazers processed\n", i+1, len(stargazers))
		}
	}

	// Step 8: Forks
	log.Println("Step 8/9: Fetching forks...")
	forks, err := githubClient.GetForks(ctx, owner, repo)
	if err != nil {
		log.Fatalf("Failed to fetch forks: %v", err)
	}
	log.Printf("Found %d forks\n", len(forks))

	for i, fork := range forks {
		githubID := fork.ID
		payload, _ := json.Marshal(fork)

		event := &feed.Event{
			Type:         feed.EventFork,
			GitHubUser:   fork.Owner.Login,
			GitHubUserID: fork.Owner.ID,
			GitHubID:     &githubID,
			Payload:      payload,
			ContentHash:  computeContentHash(payload),
			OccurredAt:   fork.CreatedAt,
		}

		if err := store.Insert(ctx, event); err != nil {
			slog.Warn("Failed to insert fork", "fork_id", fork.ID, "error", err)
		}

		if (i+1)%50 == 0 || i+1 == len(forks) {
			log.Printf("  Progress: %d/%d forks processed\n", i+1, len(forks))
		}
	}

	// Step 9: Discussions
	log.Println("Step 9/9: Fetching discussions...")
	discussions, err := graphqlClient.FetchDiscussions(ctx, owner, repo)
	if err != nil {
		log.Printf("Warning: Failed to fetch discussions: %v (skipping)", err)
	} else {
		log.Printf("Found %d discussions\n", len(discussions))

		discussionEvents := 0
		for i, discussion := range discussions {
			// Discussion creation event
			discussionNumber := discussion.Number
			discussionID := int64(discussion.Number)
			payload, _ := json.Marshal(discussion)

			event := &feed.Event{
				Type:             feed.EventDiscussionCreated,
				GitHubUser:       discussion.Author.Login,
				GitHubUserID:     0, // GraphQL doesn't provide user ID
				DiscussionNumber: &discussionNumber,
				GitHubID:         &discussionID,
				Payload:          payload,
				ContentHash:      computeContentHash(payload),
				OccurredAt:       discussion.CreatedAt,
			}

			if err := store.Insert(ctx, event); err != nil {
				slog.Warn("Failed to insert discussion", "discussion", discussion.Number, "error", err)
			} else {
				discussionEvents++
			}

			// Discussion comments
			for _, comment := range discussion.Comments {
				commentID := int64(comment.Number)
				commentPayload, _ := json.Marshal(comment)

				commentEvent := &feed.Event{
					Type:             feed.EventDiscussionComment,
					GitHubUser:       comment.Author.Login,
					GitHubUserID:     0,
					DiscussionNumber: &discussionNumber,
					CommentID:        &commentID,
					GitHubID:         &commentID,
					Payload:          commentPayload,
					ContentHash:      computeContentHash(commentPayload),
					OccurredAt:       comment.CreatedAt,
				}

				if err := store.Insert(ctx, commentEvent); err != nil {
					slog.Warn("Failed to insert discussion comment", "discussion", discussion.Number, "error", err)
				} else {
					discussionEvents++
				}
			}

			// Discussion reactions
			for _, reaction := range discussion.Reactions {
				var choice *int8
				if reaction.Content == "+1" {
					c := int8(1)
					choice = &c
				} else if reaction.Content == "-1" {
					c := int8(-1)
					choice = &c
				}

				reactionID := int64(reaction.Number)
				reactionType := reaction.Content
				reactionPayload, _ := json.Marshal(reaction)

				reactionEvent := &feed.Event{
					Type:             feed.EventReaction,
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

				if err := store.Insert(ctx, reactionEvent); err != nil {
					slog.Warn("Failed to insert discussion reaction", "discussion", discussion.Number, "error", err)
				} else {
					discussionEvents++
				}
			}

			if (i+1)%10 == 0 || i+1 == len(discussions) {
				log.Printf("  Progress: %d/%d discussions processed, %d events captured\n", i+1, len(discussions), discussionEvents)
			}
		}
		log.Printf("Total discussion events captured: %d\n", discussionEvents)
	}

	// Cleanup: deduplicate star/fork events (backfill + ingester can create duplicates)
	log.Println("Deduplicating star/fork events...")
	deduped, err := store.DeduplicateStarsForks(ctx)
	if err != nil {
		log.Printf("Warning: Failed to deduplicate stars/forks: %v\n", err)
	} else if deduped > 0 {
		log.Printf("  Removed %d duplicate star/fork events\n", deduped)
	}

	// Final summary
	log.Println("\n=== Backfill Complete ===")
	log.Printf("PRs: %d\n", len(prs))
	log.Printf("Issues: %d\n", len(issues))
	log.Printf("Comments: %d\n", len(comments))
	log.Printf("PR Reactions (VOTES): %d\n", totalReactions)
	log.Printf("Issue Reactions: %d\n", issueReactions)
	log.Printf("Comment Reactions: %d\n", commentReactions)
	log.Printf("Stargazers: %d\n", len(stargazers))
	log.Printf("Forks: %d\n", len(forks))
	if len(discussions) > 0 {
		log.Printf("Discussions: %d\n", len(discussions))
	}

	log.Println("\nBackfill completed successfully!")
}

func parseRepo(repoStr string) (owner, repo string) {
	// Simple string split
	for i, ch := range repoStr {
		if ch == '/' {
			return repoStr[:i], repoStr[i+1:]
		}
	}

	log.Fatalf("Invalid repo format: %s (expected owner/repo)", repoStr)
	return "", ""
}

func parseTime(timeStr string) time.Time {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Now()
	}
	return t
}

func computeContentHash(payload []byte) string {
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

// parseIssueNumber extracts the issue/PR number from a GitHub issue_url.
// Format: https://api.github.com/repos/{owner}/{repo}/issues/{number}
func parseIssueNumber(issueURL string) int {
	idx := strings.LastIndex(issueURL, "/")
	if idx < 0 || idx+1 >= len(issueURL) {
		return 0
	}
	n, _ := strconv.Atoi(issueURL[idx+1:])
	return n
}

func checkRateLimit(ctx context.Context, client *github.Client) {
	rateLimit, err := client.GetRateLimit(ctx)
	if err != nil {
		slog.Warn("Failed to check rate limit", "error", err)
		return
	}

	log.Printf("  Rate limit: %d/%d remaining (resets at %s)\n",
		rateLimit.Remaining, rateLimit.Limit, rateLimit.Reset.Format("15:04:05"))

	// If remaining is low, sleep until reset
	if rateLimit.Remaining < 100 {
		sleepDuration := time.Until(rateLimit.Reset).Round(time.Second)
		if sleepDuration > 0 {
			log.Printf("  Rate limit low, sleeping for %s...\n", sleepDuration)
			time.Sleep(sleepDuration + 5*time.Second) // Add 5s buffer
		}
	}
}
