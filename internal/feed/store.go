package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides database operations for events
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new event store
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Insert inserts a new event into the database.
// Deduplication: ON CONFLICT (github_id) catches exact ID matches.
// The WHERE NOT EXISTS clause catches content duplicates that differ
// only in github_id (e.g. legacy NULL-github_id rows vs new rows).
func (s *Store) Insert(ctx context.Context, event *Event) error {
	query := `
		WITH new_event (
			type, github_user, github_user_id,
			pr_number, issue_number, discussion_number, comment_id,
			choice, reaction_type, github_id, payload, content_hash,
			occurred_at
		) AS (
			VALUES ($1::varchar, $2::varchar, $3::bigint,
				$4::int, $5::int, $6::int, $7::bigint,
				$8::smallint, $9::varchar, $10::bigint, $11::jsonb, $12::varchar,
				$13::timestamptz)
		)
		INSERT INTO events (
			type, github_user, github_user_id,
			pr_number, issue_number, discussion_number, comment_id,
			choice, reaction_type, github_id, payload, content_hash,
			occurred_at
		)
		SELECT * FROM new_event n
		WHERE NOT EXISTS (
			SELECT 1 FROM events e
			WHERE e.content_hash = n.content_hash
			  AND e.type = n.type
			  AND e.github_user = n.github_user
			  AND e.occurred_at = n.occurred_at
		)
		-- Stars and forks: one per user (backfill and ingester use different github_ids)
		AND NOT EXISTS (
			SELECT 1 FROM events e
			WHERE e.type = n.type
			  AND e.github_user = n.github_user
			  AND n.type IN ('star', 'fork')
		)
		ON CONFLICT (github_id) DO NOTHING
		RETURNING id, ingested_at
	`

	err := s.pool.QueryRow(
		ctx, query,
		event.Type, event.GitHubUser, event.GitHubUserID,
		event.PRNumber, event.IssueNumber, event.DiscussionNumber, event.CommentID,
		event.Choice, event.ReactionType, event.GitHubID, event.Payload, event.ContentHash,
		event.OccurredAt,
	).Scan(&event.ID, &event.IngestedAt)

	if err != nil {
		// If no rows returned due to conflict, it's not an error - it's a duplicate
		if err == pgx.ErrNoRows {
			return nil
		}
		return fmt.Errorf("failed to insert event: %w", err)
	}

	return nil
}

// eventColumns is the standard column list for event queries
const eventColumns = `id, type, github_user, github_user_id,
			pr_number, issue_number, discussion_number, comment_id,
			choice, reaction_type, github_id, payload, content_hash,
			edit_history, occurred_at, ingested_at`

// scanEvent scans a row into an Event struct
func scanEvent(row pgx.Row) (*Event, error) {
	event := &Event{}
	err := row.Scan(
		&event.ID, &event.Type, &event.GitHubUser, &event.GitHubUserID,
		&event.PRNumber, &event.IssueNumber, &event.DiscussionNumber, &event.CommentID,
		&event.Choice, &event.ReactionType, &event.GitHubID, &event.Payload, &event.ContentHash,
		&event.EditHistory, &event.OccurredAt, &event.IngestedAt,
	)
	return event, err
}

// scanEvents scans multiple rows into Event structs
func scanEvents(rows pgx.Rows) ([]*Event, error) {
	events := []*Event{}
	for rows.Next() {
		event := &Event{}
		err := rows.Scan(
			&event.ID, &event.Type, &event.GitHubUser, &event.GitHubUserID,
			&event.PRNumber, &event.IssueNumber, &event.DiscussionNumber, &event.CommentID,
			&event.Choice, &event.ReactionType, &event.GitHubID, &event.Payload, &event.ContentHash,
			&event.EditHistory, &event.OccurredAt, &event.IngestedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}
	return events, nil
}

// UpdateCommentEdit updates a comment's payload and appends the previous body to edit_history
func (s *Store) UpdateCommentEdit(ctx context.Context, commentID int64, newPayload []byte, previousBody string, editedAt time.Time) error {
	editEntry, _ := json.Marshal([]EditHistoryEntry{{Body: previousBody, EditedAt: editedAt}})

	query := `
		UPDATE events
		SET payload = $2,
			content_hash = $3,
			edit_history = $4::jsonb || edit_history
		WHERE comment_id = $1
	`

	tag, err := s.pool.Exec(ctx, query, commentID, newPayload, computeContentHash(newPayload), editEntry)
	if err != nil {
		return fmt.Errorf("failed to update comment edit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("comment %d not found", commentID)
	}
	return nil
}

// DeduplicateStarsForks removes duplicate star/fork events, keeping the earliest per user.
func (s *Store) DeduplicateStarsForks(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM events
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY type, github_user ORDER BY occurred_at ASC) as rn
				FROM events
				WHERE type IN ('star', 'fork')
			) sub
			WHERE sub.rn > 1
		)
	`
	tag, err := s.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to deduplicate stars/forks: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteByType removes all events of a given type. Returns the number of rows deleted.
func (s *Store) DeleteByType(ctx context.Context, eventType EventType) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM events WHERE type = $1`, eventType)
	if err != nil {
		return 0, fmt.Errorf("failed to delete events by type: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteByTypes removes all events matching any of the given types. Returns total rows deleted.
func (s *Store) DeleteByTypes(ctx context.Context, types []EventType) (int64, error) {
	typeStrs := make([]string, len(types))
	for i, t := range types {
		typeStrs[i] = string(t)
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM events WHERE type = ANY($1)`, typeStrs)
	if err != nil {
		return 0, fmt.Errorf("failed to delete events by types: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteByCommentID removes a comment event when it gets deleted on GitHub
func (s *Store) DeleteByCommentID(ctx context.Context, commentID int64) error {
	query := `DELETE FROM events WHERE comment_id = $1`
	tag, err := s.pool.Exec(ctx, query, commentID)
	if err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("comment %d not found", commentID)
	}
	return nil
}

// GetByID retrieves an event by its ID
func (s *Store) GetByID(ctx context.Context, id string) (*Event, error) {
	query := fmt.Sprintf(`SELECT %s FROM events WHERE id = $1`, eventColumns)

	event, err := scanEvent(s.pool.QueryRow(ctx, query, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("event not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	return event, nil
}

// ListFilters contains filter criteria for listing events
type ListFilters struct {
	Types                   []EventType
	PRNumber                *int
	GitHubUser              *string
	Since                   *time.Time
	Until                   *time.Time
	ExcludeCommentReactions bool // Hide reaction events that target comments (not PR/issue votes)
}

// List retrieves events with optional filters, sorting, and pagination
func (s *Store) List(ctx context.Context, filters *ListFilters, sort string, limit int, cursor *string) ([]*Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.listInternal(ctx, filters, sort, limit, cursor)
}

// listInternal is the shared implementation for List and ExportList
func (s *Store) listInternal(ctx context.Context, filters *ListFilters, sort string, limit int, cursor *string) ([]*Event, error) {
	query := fmt.Sprintf(`SELECT %s FROM events WHERE 1=1`, eventColumns)

	args := []interface{}{}
	argPos := 1

	// Apply filters
	if filters != nil {
		if len(filters.Types) > 0 {
			query += fmt.Sprintf(" AND type = ANY($%d)", argPos)
			args = append(args, filters.Types)
			argPos++
		}
		if filters.PRNumber != nil {
			query += fmt.Sprintf(" AND pr_number = $%d", argPos)
			args = append(args, *filters.PRNumber)
			argPos++
		}
		if filters.GitHubUser != nil {
			query += fmt.Sprintf(" AND github_user = $%d", argPos)
			args = append(args, *filters.GitHubUser)
			argPos++
		}
		if filters.Since != nil {
			query += fmt.Sprintf(" AND occurred_at >= $%d", argPos)
			args = append(args, *filters.Since)
			argPos++
		}
		if filters.Until != nil {
			query += fmt.Sprintf(" AND occurred_at <= $%d", argPos)
			args = append(args, *filters.Until)
			argPos++
		}
		if filters.ExcludeCommentReactions {
			query += " AND NOT (type = 'reaction' AND comment_id IS NOT NULL)"
		}
	}

	// Apply cursor for pagination (direction depends on sort)
	if cursor != nil && *cursor != "" {
		op := "<" // newest: get events before cursor
		if sort == "oldest" {
			op = ">" // oldest: get events after cursor
		}
		query += fmt.Sprintf(
			" AND (occurred_at, id) %s (SELECT occurred_at, id FROM events WHERE id = $%d)",
			op, argPos,
		)
		args = append(args, *cursor)
		argPos++
	}

	// Apply sorting
	switch sort {
	case "oldest":
		query += " ORDER BY occurred_at ASC, id ASC"
	default: // "newest"
		query += " ORDER BY occurred_at DESC, id DESC"
	}

	// Apply limit
	query += fmt.Sprintf(" LIMIT $%d", len(args)+1)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// ExportList retrieves events for bulk export with larger page sizes (max 1000).
// Designed for research use — supports streaming large datasets via cursor pagination.
func (s *Store) ExportList(ctx context.Context, filters *ListFilters, sort string, limit int, cursor *string) ([]*Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	return s.listInternal(ctx, filters, sort, limit, cursor)
}

// GetByPR retrieves events for a specific PR (capped at 500)
func (s *Store) GetByPR(ctx context.Context, prNumber int) ([]*Event, error) {
	query := fmt.Sprintf(`SELECT %s FROM events WHERE pr_number = $1 ORDER BY occurred_at DESC LIMIT 500`, eventColumns)

	rows, err := s.pool.Query(ctx, query, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get events for PR: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// GetByUser retrieves events for a specific GitHub user (capped at 500)
func (s *Store) GetByUser(ctx context.Context, githubUser string) ([]*Event, error) {
	query := fmt.Sprintf(`SELECT %s FROM events WHERE github_user = $1 ORDER BY occurred_at DESC LIMIT 500`, eventColumns)

	rows, err := s.pool.Query(ctx, query, githubUser)
	if err != nil {
		return nil, fmt.Errorf("failed to get events for user: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// GetVoters retrieves aggregated voting statistics for all voters
func (s *Store) GetVoters(ctx context.Context) ([]*VoterSummary, error) {
	query := `
		SELECT
			github_user,
			github_user_id,
			COUNT(*) as total_votes,
			COUNT(*) FILTER (WHERE choice = 1) as upvotes,
			COUNT(*) FILTER (WHERE choice = -1) as downvotes,
			MIN(occurred_at) as first_vote,
			MAX(occurred_at) as last_vote,
			array_agg(DISTINCT pr_number ORDER BY pr_number) FILTER (WHERE pr_number IS NOT NULL) as prs_voted_on
		FROM events
		WHERE type = 'reaction' AND choice IS NOT NULL AND comment_id IS NULL
		GROUP BY github_user, github_user_id
		ORDER BY total_votes DESC
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get voters: %w", err)
	}
	defer rows.Close()

	voters := []*VoterSummary{}
	for rows.Next() {
		voter := &VoterSummary{}

		err := rows.Scan(
			&voter.GitHubUser,
			&voter.GitHubUserID,
			&voter.TotalVotes,
			&voter.Upvotes,
			&voter.Downvotes,
			&voter.FirstVote,
			&voter.LastVote,
			&voter.PRsVotedOn,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan voter: %w", err)
		}

		if voter.PRsVotedOn == nil {
			voter.PRsVotedOn = []int{}
		}
		voter.UniquePRs = len(voter.PRsVotedOn)

		voters = append(voters, voter)
	}

	return voters, nil
}

// GetPRVotes retrieves vote breakdown for a specific PR
func (s *Store) GetPRVotes(ctx context.Context, prNumber int) (upvotes int, downvotes int, err error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE choice = 1) as upvotes,
			COUNT(*) FILTER (WHERE choice = -1) as downvotes
		FROM events
		WHERE type = 'reaction' AND pr_number = $1 AND choice IS NOT NULL AND comment_id IS NULL
	`

	err = s.pool.QueryRow(ctx, query, prNumber).Scan(&upvotes, &downvotes)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get PR votes: %w", err)
	}

	return upvotes, downvotes, nil
}

// Stats represents feed statistics
type Stats struct {
	TotalEvents    int
	TotalVotes     int
	TotalVoters    int
	LatestEventAt  *time.Time
	EventsByType   map[string]int
	EventsLastHour int
}

// GetStats retrieves aggregate statistics for the feed
func (s *Store) GetStats(ctx context.Context) (*Stats, error) {
	query := `
		SELECT
			COUNT(*) as total_events,
			COUNT(*) FILTER (WHERE type = 'reaction' AND choice IS NOT NULL AND comment_id IS NULL) as total_votes,
			COUNT(DISTINCT github_user) FILTER (WHERE type = 'reaction' AND choice IS NOT NULL AND comment_id IS NULL) as total_voters,
			MAX(occurred_at) as latest_event,
			COUNT(*) FILTER (WHERE occurred_at > NOW() - INTERVAL '1 hour') as events_last_hour
		FROM events
	`

	stats := &Stats{
		EventsByType: make(map[string]int),
	}

	err := s.pool.QueryRow(ctx, query).Scan(
		&stats.TotalEvents,
		&stats.TotalVotes,
		&stats.TotalVoters,
		&stats.LatestEventAt,
		&stats.EventsLastHour,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	// Get events by type
	typeQuery := `
		SELECT type, COUNT(*) as count
		FROM events
		GROUP BY type
		ORDER BY count DESC
	`

	rows, err := s.pool.Query(ctx, typeQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get events by type: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan event type: %w", err)
		}
		stats.EventsByType[eventType] = count
	}

	return stats, nil
}

// GetByIssue retrieves events for a specific issue (capped at 500)
func (s *Store) GetByIssue(ctx context.Context, issueNumber int) ([]*Event, error) {
	query := fmt.Sprintf(`SELECT %s FROM events WHERE issue_number = $1 ORDER BY occurred_at DESC LIMIT 500`, eventColumns)

	rows, err := s.pool.Query(ctx, query, issueNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get events for issue: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// GetVoter retrieves aggregated voting statistics for a single voter
func (s *Store) GetVoter(ctx context.Context, githubUser string) (*VoterSummary, error) {
	query := `
		SELECT
			github_user,
			github_user_id,
			COUNT(*) as total_votes,
			COUNT(*) FILTER (WHERE choice = 1) as upvotes,
			COUNT(*) FILTER (WHERE choice = -1) as downvotes,
			MIN(occurred_at) as first_vote,
			MAX(occurred_at) as last_vote,
			array_agg(DISTINCT pr_number ORDER BY pr_number) FILTER (WHERE pr_number IS NOT NULL) as prs_voted_on
		FROM events
		WHERE type = 'reaction' AND choice IS NOT NULL AND comment_id IS NULL AND github_user = $1
		GROUP BY github_user, github_user_id
	`

	voter := &VoterSummary{}

	err := s.pool.QueryRow(ctx, query, githubUser).Scan(
		&voter.GitHubUser,
		&voter.GitHubUserID,
		&voter.TotalVotes,
		&voter.Upvotes,
		&voter.Downvotes,
		&voter.FirstVote,
		&voter.LastVote,
		&voter.PRsVotedOn,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("voter not found: %s", githubUser)
		}
		return nil, fmt.Errorf("failed to get voter: %w", err)
	}

	if voter.PRsVotedOn == nil {
		voter.PRsVotedOn = []int{}
	}
	voter.UniquePRs = len(voter.PRsVotedOn)

	return voter, nil
}

// VoteDetail represents detailed vote information
type VoteDetail struct {
	GitHubUser   string
	GitHubUserID int64
	Choice       int8
	OccurredAt   time.Time
}

// GetPRVoteDetails retrieves detailed vote information for a PR
func (s *Store) GetPRVoteDetails(ctx context.Context, prNumber int) ([]*VoteDetail, error) {
	query := `
		SELECT github_user, github_user_id, choice, occurred_at
		FROM events
		WHERE type = 'reaction' AND pr_number = $1 AND choice IS NOT NULL AND comment_id IS NULL
		ORDER BY occurred_at ASC
	`

	rows, err := s.pool.Query(ctx, query, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR vote details: %w", err)
	}
	defer rows.Close()

	details := []*VoteDetail{}
	for rows.Next() {
		detail := &VoteDetail{}
		err := rows.Scan(
			&detail.GitHubUser,
			&detail.GitHubUserID,
			&detail.Choice,
			&detail.OccurredAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan vote detail: %w", err)
		}
		details = append(details, detail)
	}

	return details, nil
}

// GetCommentReactionCounts returns aggregated reaction counts per comment ID.
// Returns map[commentID] -> map[reactionType] -> count.
func (s *Store) GetCommentReactionCounts(ctx context.Context, commentIDs []int64) (map[int64]map[string]int, error) {
	if len(commentIDs) == 0 {
		return nil, nil
	}

	query := `
		SELECT comment_id, reaction_type, COUNT(*) as cnt
		FROM events
		WHERE type = 'reaction' AND comment_id = ANY($1) AND reaction_type IS NOT NULL
		GROUP BY comment_id, reaction_type
	`

	rows, err := s.pool.Query(ctx, query, commentIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get comment reaction counts: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]map[string]int)
	for rows.Next() {
		var commentID int64
		var reactionType string
		var count int
		if err := rows.Scan(&commentID, &reactionType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan reaction count: %w", err)
		}
		if result[commentID] == nil {
			result[commentID] = make(map[string]int)
		}
		result[commentID][reactionType] = count
	}

	return result, nil
}

// GetPRReactionCounts returns aggregated reaction counts per PR number.
// Only counts PR-level reactions (comment_id IS NULL), not comment reactions.
// Returns map[prNumber] -> map[reactionType] -> count.
func (s *Store) GetPRReactionCounts(ctx context.Context, prNumbers []int) (map[int]map[string]int, error) {
	if len(prNumbers) == 0 {
		return nil, nil
	}

	query := `
		SELECT pr_number, reaction_type, COUNT(*) as cnt
		FROM events
		WHERE type = 'reaction' AND pr_number = ANY($1) AND comment_id IS NULL AND reaction_type IS NOT NULL
		GROUP BY pr_number, reaction_type
	`

	rows, err := s.pool.Query(ctx, query, prNumbers)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR reaction counts: %w", err)
	}
	defer rows.Close()

	result := make(map[int]map[string]int)
	for rows.Next() {
		var prNumber int
		var reactionType string
		var count int
		if err := rows.Scan(&prNumber, &reactionType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan PR reaction count: %w", err)
		}
		if result[prNumber] == nil {
			result[prNumber] = make(map[string]int)
		}
		result[prNumber][reactionType] = count
	}

	return result, nil
}

// Count returns the total number of events matching the filters
func (s *Store) Count(ctx context.Context, filters *ListFilters) (int, error) {
	query := `SELECT COUNT(*) FROM events WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if filters != nil {
		if len(filters.Types) > 0 {
			query += fmt.Sprintf(" AND type = ANY($%d)", argPos)
			args = append(args, filters.Types)
			argPos++
		}
		if filters.PRNumber != nil {
			query += fmt.Sprintf(" AND pr_number = $%d", argPos)
			args = append(args, *filters.PRNumber)
			argPos++
		}
		if filters.GitHubUser != nil {
			query += fmt.Sprintf(" AND github_user = $%d", argPos)
			args = append(args, *filters.GitHubUser)
			argPos++
		}
		if filters.Since != nil {
			query += fmt.Sprintf(" AND occurred_at >= $%d", argPos)
			args = append(args, *filters.Since)
			argPos++
		}
		if filters.Until != nil {
			query += fmt.Sprintf(" AND occurred_at <= $%d", argPos)
			args = append(args, *filters.Until)
			argPos++
		}
		if filters.ExcludeCommentReactions {
			query += " AND NOT (type = 'reaction' AND comment_id IS NOT NULL)"
		}
	}

	var count int
	err := s.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count events: %w", err)
	}

	return count, nil
}
