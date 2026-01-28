package github

import (
	"encoding/json"
	"time"
)

// RawGitHubEvent represents the raw event structure from the GitHub Events API
type RawGitHubEvent struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Actor     EventActor      `json:"actor"`
	Repo      EventRepo       `json:"repo"`
	Payload   json.RawMessage `json:"payload"`
	Public    bool            `json:"public"`
	CreatedAt time.Time       `json:"created_at"`
}

// EventActor represents the user who triggered the event
type EventActor struct {
	ID           int64  `json:"id"`
	Login        string `json:"login"`
	DisplayLogin string `json:"display_login"`
	GravatarID   string `json:"gravatar_id"`
	URL          string `json:"url"`
	AvatarURL    string `json:"avatar_url"`
}

// EventRepo represents the repository in the event
type EventRepo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Event Payload Types - Each event type has a specific payload structure

// PullRequestEventPayload for PullRequestEvent
type PullRequestEventPayload struct {
	Action      string `json:"action"` // opened, closed, reopened, edited, synchronize
	Number      int    `json:"number"`
	PullRequest struct {
		ID        int64  `json:"id"`
		Number    int    `json:"number"`
		State     string `json:"state"`
		Title     string `json:"title"`
		User      struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"user"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Merged    bool      `json:"merged"`
		MergedAt  *time.Time `json:"merged_at"`
	} `json:"pull_request"`
}

// IssueCommentEventPayload for IssueCommentEvent
type IssueCommentEventPayload struct {
	Action  string `json:"action"` // created, edited, deleted
	Issue   struct {
		ID              int64  `json:"id"`
		Number          int    `json:"number"`
		Title           string `json:"title"`
		PullRequest     *struct{} `json:"pull_request"` // Present if comment is on a PR
	} `json:"issue"`
	Comment struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		User      struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"user"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"comment"`
}

// PushEventPayload for PushEvent
type PushEventPayload struct {
	Ref     string `json:"ref"` // refs/heads/branch-name
	Before  string `json:"before"`
	After   string `json:"after"`
	Commits []struct {
		SHA     string `json:"sha"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"commits"`
	Size int `json:"size"`
}

// WatchEventPayload for WatchEvent (starring)
type WatchEventPayload struct {
	Action string `json:"action"` // started (no "stopped" event exists)
}

// ForkEventPayload for ForkEvent
type ForkEventPayload struct {
	Forkee struct {
		ID        int64     `json:"id"`
		Name      string    `json:"name"`
		FullName  string    `json:"full_name"`
		Owner     struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"owner"`
		CreatedAt time.Time `json:"created_at"`
	} `json:"forkee"`
}

// IssuesEventPayload for IssuesEvent
type IssuesEventPayload struct {
	Action string `json:"action"` // opened, closed, reopened, edited
	Issue  struct {
		ID        int64     `json:"id"`
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		State     string    `json:"state"`
		User      struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"user"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"issue"`
}

// PullRequestReviewEventPayload for PullRequestReviewEvent
type PullRequestReviewEventPayload struct {
	Action      string `json:"action"` // submitted, edited, dismissed
	Review      struct {
		ID          int64  `json:"id"`
		User        struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"user"`
		Body        string    `json:"body"`
		State       string    `json:"state"` // approved, changes_requested, commented
		SubmittedAt time.Time `json:"submitted_at"`
	} `json:"review"`
	PullRequest struct {
		Number int `json:"number"`
	} `json:"pull_request"`
}

// PullRequestReviewCommentEventPayload for PullRequestReviewCommentEvent
type PullRequestReviewCommentEventPayload struct {
	Action      string `json:"action"` // created, edited, deleted
	Comment     struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		User      struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"user"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"comment"`
	PullRequest struct {
		Number int `json:"number"`
	} `json:"pull_request"`
}

// CreateEventPayload for CreateEvent
type CreateEventPayload struct {
	Ref        string `json:"ref"`          // branch or tag name
	RefType    string `json:"ref_type"`     // branch, tag, or repository
	MasterBranch string `json:"master_branch"`
}

// DeleteEventPayload for DeleteEvent
type DeleteEventPayload struct {
	Ref     string `json:"ref"`      // branch or tag name
	RefType string `json:"ref_type"` // branch or tag
}

// ReleaseEventPayload for ReleaseEvent
type ReleaseEventPayload struct {
	Action  string `json:"action"` // published, edited, deleted
	Release struct {
		ID          int64     `json:"id"`
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
		CreatedAt   time.Time `json:"created_at"`
		PublishedAt time.Time `json:"published_at"`
	} `json:"release"`
}

// GollumEventPayload for GollumEvent (wiki)
type GollumEventPayload struct {
	Pages []struct {
		PageName string `json:"page_name"`
		Title    string `json:"title"`
		Action   string `json:"action"` // created, edited
		SHA      string `json:"sha"`
	} `json:"pages"`
}

// MemberEventPayload for MemberEvent
type MemberEventPayload struct {
	Action string `json:"action"` // added, removed, edited
	Member struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"member"`
}

// CommitCommentEventPayload for CommitCommentEvent
type CommitCommentEventPayload struct {
	Comment struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		User      struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"user"`
		CommitID  string    `json:"commit_id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"comment"`
}

// DiscussionEventPayload for DiscussionEvent
type DiscussionEventPayload struct {
	Action     string `json:"action"` // created, edited, deleted, etc.
	Discussion struct {
		ID        int64     `json:"id"`
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		User      struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"user"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"discussion"`
}

// PublicEventPayload for PublicEvent (repository made public)
// This event has an empty payload
type PublicEventPayload struct{}
