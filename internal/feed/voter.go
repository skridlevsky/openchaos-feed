package feed

import "time"

// VoterSummary represents aggregated voting statistics for a user
// Used for Sybil resistance research and behavioral analysis
type VoterSummary struct {
	GitHubUser   string    `json:"githubUser"`
	GitHubUserID int64     `json:"githubUserId"`
	TotalVotes   int       `json:"totalVotes"`
	Upvotes      int       `json:"upvotes"`
	Downvotes    int       `json:"downvotes"`
	FirstVote    time.Time `json:"firstVote"`
	LastVote     time.Time `json:"lastVote"`
	PRsVotedOn   []int     `json:"prsVotedOn"`
	UniquePRs    int       `json:"uniquePrs"`
}
