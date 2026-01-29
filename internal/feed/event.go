package feed

import (
	"encoding/json"
	"time"
)

// EventType represents the type of GitHub activity
type EventType string

// Event type constants
const (
	// PR Lifecycle
	EventPROpened       EventType = "pr_opened"
	EventPRClosed       EventType = "pr_closed"
	EventPRMerged       EventType = "pr_merged"
	EventPRReopened     EventType = "pr_reopened"
	EventPREdited       EventType = "pr_edited"
	EventPRSynchronized EventType = "pr_synchronized"

	// PR Reviews
	EventReviewSubmitted EventType = "review_submitted"
	EventReviewComment   EventType = "review_comment"
	EventReviewDismissed EventType = "review_dismissed"

	// Issue Lifecycle
	EventIssueOpened   EventType = "issue_opened"
	EventIssueClosed   EventType = "issue_closed"
	EventIssueReopened EventType = "issue_reopened"
	EventIssueEdited   EventType = "issue_edited"

	// Comments
	EventIssueComment      EventType = "issue_comment"
	EventCommitComment     EventType = "commit_comment"
	EventDiscussionComment EventType = "discussion_comment"

	// Reactions (votes and engagement)
	EventReaction EventType = "reaction"

	// Repository
	EventStar    EventType = "star"
	EventFork    EventType = "fork"
	EventPush    EventType = "push"
	EventRelease EventType = "release"

	// Branches/Tags
	EventBranchCreated EventType = "branch_created"
	EventBranchDeleted EventType = "branch_deleted"
	EventTagCreated    EventType = "tag_created"
	EventTagDeleted    EventType = "tag_deleted"

	// Discussions
	EventDiscussionCreated  EventType = "discussion_created"
	EventDiscussionAnswered EventType = "discussion_answered"

	// Wiki
	EventWikiEdit EventType = "wiki_edit"

	// Membership
	EventCollaboratorAdded EventType = "collaborator_added"
)

// Event represents a GitHub activity event
type Event struct {
	ID               string          `json:"id"`
	Type             EventType       `json:"type"`
	GitHubUser       string          `json:"githubUser"`
	GitHubUserID     int64           `json:"githubUserId"`
	PRNumber         *int            `json:"prNumber,omitempty"`
	IssueNumber      *int            `json:"issueNumber,omitempty"`
	DiscussionNumber *int            `json:"discussionNumber,omitempty"`
	CommentID        *int64          `json:"commentId,omitempty"`
	Choice           *int8           `json:"choice,omitempty"` // +1 or -1 for votes
	ReactionType     *string         `json:"reactionType,omitempty"`
	GitHubID         *int64          `json:"githubId,omitempty"`
	Payload          json.RawMessage `json:"payload"`
	ContentHash      string          `json:"contentHash"`
	EditHistory      json.RawMessage `json:"editHistory"`
	OccurredAt       time.Time       `json:"occurredAt"`
	IngestedAt       time.Time       `json:"ingestedAt"`
	ReactionSummary  map[string]int  `json:"reactionSummary,omitempty"` // Populated post-query for comment events
}

// EditHistoryEntry records a previous version of comment body before an edit
type EditHistoryEntry struct {
	Body     string    `json:"body"`
	EditedAt time.Time `json:"editedAt"`
}
