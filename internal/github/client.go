package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client wraps the GitHub API client
type Client struct {
	token      string
	httpClient *http.Client
	cache      *PRCache
}

// NewClient creates a new GitHub API client
func NewClient(token string, cache *PRCache) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: cache,
	}
}

// doRequest makes an authenticated request to the GitHub API
func (c *Client) doRequest(ctx context.Context, method, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add auth header if token is configured
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "OpenChaos-Token-Gov")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Check rate limit
	if resp.StatusCode == http.StatusForbidden {
		if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining == "0" {
			resetTime := resp.Header.Get("X-RateLimit-Reset")
			resp.Body.Close()
			return nil, fmt.Errorf("rate limit exceeded, resets at: %s", resetTime)
		}
	}

	return resp, nil
}

// doRequestWithETag makes a request with optional ETag for caching
func (c *Client) doRequestWithETag(ctx context.Context, method, url string, etag *string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add auth header if token is configured
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "OpenChaos-Token-Gov")

	// Add ETag for conditional requests
	if etag != nil && *etag != "" {
		req.Header.Set("If-None-Match", *etag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Check rate limit
	if resp.StatusCode == http.StatusForbidden {
		if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining == "0" {
			resetTime := resp.Header.Get("X-RateLimit-Reset")
			resp.Body.Close()
			return nil, fmt.Errorf("rate limit exceeded, resets at: %s", resetTime)
		}
	}

	return resp, nil
}

// readAndClose reads the body and closes it. Use in paginated loops
// instead of defer resp.Body.Close() to avoid leaking connections.
func readAndClose(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(target)
}

// readErrorAndClose reads an error body and closes it.
func readErrorAndClose(resp *http.Response) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("github API error %d: %s", resp.StatusCode, string(body))
}

// GitHubPR represents a PR from GitHub API
type GitHubPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	User      struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Merged    bool   `json:"merged"`
}

// GetOpenPRs fetches all open PRs for a repository
func (c *Client) GetOpenPRs(ctx context.Context, owner, repo string) ([]*PR, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=open&per_page=100", owner, repo)

	resp, err := c.doRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github API error %d: %s", resp.StatusCode, string(body))
	}

	var ghPRs []GitHubPR
	if err := json.NewDecoder(resp.Body).Decode(&ghPRs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to our PR type and cache
	prs := make([]*PR, len(ghPRs))
	for i, ghPR := range ghPRs {
		pr := &PR{
			Number:       ghPR.Number,
			Title:        ghPR.Title,
			State:        ghPR.State,
			Author:       ghPR.User.Login,
			AuthorAvatar: ghPR.User.AvatarURL,
			URL:          ghPR.HTMLURL,
			CreatedAt:    ghPR.CreatedAt,
			UpdatedAt:    ghPR.UpdatedAt,
			Merged:       ghPR.Merged,
		}
		prs[i] = pr

		// Update cache
		if c.cache != nil {
			c.cache.UpdatePR(pr)
		}
	}

	return prs, nil
}

// GetPR fetches a single PR by number
func (c *Client) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	// Check cache first
	if c.cache != nil {
		if pr, found := c.cache.GetPR(number); found {
			return pr, nil
		}
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, number)

	resp, err := c.doRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("PR #%d not found", number)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github API error %d: %s", resp.StatusCode, string(body))
	}

	var ghPR GitHubPR
	if err := json.NewDecoder(resp.Body).Decode(&ghPR); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	pr := &PR{
		Number:       ghPR.Number,
		Title:        ghPR.Title,
		State:        ghPR.State,
		Author:       ghPR.User.Login,
		AuthorAvatar: ghPR.User.AvatarURL,
		URL:          ghPR.HTMLURL,
		CreatedAt:    ghPR.CreatedAt,
		UpdatedAt:    ghPR.UpdatedAt,
		Merged:       ghPR.Merged,
	}

	// Update cache
	if c.cache != nil {
		c.cache.UpdatePR(pr)
	}

	return pr, nil
}

// Reaction represents a GitHub reaction
type Reaction struct {
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Content string `json:"content"` // +1, -1, laugh, hooray, confused, heart, rocket, eyes
}

// Reactions holds reaction counts
type Reactions struct {
	ThumbsUp   int      `json:"thumbsUp"`
	ThumbsDown int      `json:"thumbsDown"`
	Total      int      `json:"total"`
	Users      []string `json:"users"` // Users who reacted
}

// GetReactions fetches reactions for a PR
func (c *Client) GetReactions(ctx context.Context, owner, repo string, number int) (*Reactions, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/reactions?per_page=100", owner, repo, number)

	resp, err := c.doRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github API error %d: %s", resp.StatusCode, string(body))
	}

	var reactions []Reaction
	if err := json.NewDecoder(resp.Body).Decode(&reactions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Count reactions
	result := &Reactions{
		Users: make([]string, 0),
	}

	seen := make(map[string]bool)
	for _, reaction := range reactions {
		// Track unique users
		if !seen[reaction.User.Login] {
			seen[reaction.User.Login] = true
			result.Users = append(result.Users, reaction.User.Login)
		}

		// Count by type
		switch reaction.Content {
		case "+1":
			result.ThumbsUp++
		case "-1":
			result.ThumbsDown++
		}
	}

	result.Total = result.ThumbsUp + result.ThumbsDown

	return result, nil
}

// RateLimit holds GitHub rate limit info
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

// GetRateLimit fetches current rate limit status
func (c *Client) GetRateLimit(ctx context.Context) (*RateLimit, error) {
	url := "https://api.github.com/rate_limit"

	resp, err := c.doRequest(ctx, "GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Rate struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"rate"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &RateLimit{
		Limit:     result.Rate.Limit,
		Remaining: result.Rate.Remaining,
		Reset:     time.Unix(result.Rate.Reset, 0),
	}, nil
}

// GetRateLimitFromHeaders extracts rate limit info from response headers
func GetRateLimitFromHeaders(headers http.Header) *RateLimit {
	limit, _ := strconv.Atoi(headers.Get("X-RateLimit-Limit"))
	remaining, _ := strconv.Atoi(headers.Get("X-RateLimit-Remaining"))
	reset, _ := strconv.ParseInt(headers.Get("X-RateLimit-Reset"), 10, 64)

	return &RateLimit{
		Limit:     limit,
		Remaining: remaining,
		Reset:     time.Unix(reset, 0),
	}
}

// GetRepoEvents fetches repository events from the GitHub Events API.
// Paginates through all available pages (GitHub keeps up to 300 events, 10 pages).
// Returns events, response headers from the first page (for ETag caching), and error.
func (c *Client) GetRepoEvents(ctx context.Context, owner, repo string, etag *string) ([]RawGitHubEvent, http.Header, error) {
	firstURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/events?per_page=100", owner, repo)

	resp, err := c.doRequestWithETag(ctx, "GET", firstURL, etag)
	if err != nil {
		return nil, nil, err
	}

	// If 304 Not Modified, no new events
	if resp.StatusCode == http.StatusNotModified {
		headers := resp.Header
		resp.Body.Close()
		return nil, headers, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.Header, readErrorAndClose(resp)
	}

	var allEvents []RawGitHubEvent
	if err := json.NewDecoder(resp.Body).Decode(&allEvents); err != nil {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	firstHeaders := resp.Header
	linkHeader := resp.Header.Get("Link")
	resp.Body.Close()

	// Paginate: follow Link rel="next" headers (max 10 pages per GitHub docs)
	for page := 2; page <= 10; page++ {
		nextURL := parseLinkNext(linkHeader)
		if nextURL == "" {
			break
		}

		resp, err = c.doRequest(ctx, "GET", nextURL)
		if err != nil {
			break // Partial results are fine
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			break
		}

		var pageEvents []RawGitHubEvent
		if err := json.NewDecoder(resp.Body).Decode(&pageEvents); err != nil {
			resp.Body.Close()
			break
		}
		linkHeader = resp.Header.Get("Link")
		resp.Body.Close()

		if len(pageEvents) == 0 {
			break
		}

		allEvents = append(allEvents, pageEvents...)
	}

	return allEvents, firstHeaders, nil
}

// parseLinkNext extracts the "next" URL from a GitHub Link header.
// Format: <https://api.github.com/...?page=2>; rel="next", <...>; rel="last"
func parseLinkNext(header string) string {
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, `rel="next"`) {
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start >= 0 && end > start {
				return part[start+1 : end]
			}
		}
	}
	return ""
}

// GetIssueReactions fetches all reactions for an issue/PR with pagination.
// GitHub returns max 100 per page; this follows Link: rel="next" headers.
func (c *Client) GetIssueReactions(ctx context.Context, owner, repo string, number int) ([]DetailedReaction, error) {
	return c.fetchAllReactions(ctx, fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/reactions?per_page=100", owner, repo, number))
}

// DetailedReaction represents a reaction with full details for feed ingestion
type DetailedReaction struct {
	ID        int64  `json:"id"`
	Content   string `json:"content"` // +1, -1, laugh, hooray, confused, heart, rocket, eyes
	User      struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

// GetAllPRs fetches all PRs (open and closed) with pagination
func (c *Client) GetAllPRs(ctx context.Context, owner, repo string) ([]*GitHubPR, error) {
	allPRs := []*GitHubPR{}
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=all&per_page=%d&page=%d",
			owner, repo, perPage, page)

		resp, err := c.doRequest(ctx, "GET", url)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, readErrorAndClose(resp)
		}

		var prs []GitHubPR
		if err := readAndClose(resp, &prs); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		if len(prs) == 0 {
			break
		}

		for i := range prs {
			allPRs = append(allPRs, &prs[i])
		}

		if len(prs) < perPage {
			break
		}

		page++
	}

	return allPRs, nil
}

// GetAllIssues fetches all issues (open and closed) with pagination
func (c *Client) GetAllIssues(ctx context.Context, owner, repo string) ([]GitHubIssue, error) {
	allIssues := []GitHubIssue{}
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?state=all&per_page=%d&page=%d",
			owner, repo, perPage, page)

		resp, err := c.doRequest(ctx, "GET", url)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, readErrorAndClose(resp)
		}

		var issues []GitHubIssue
		if err := readAndClose(resp, &issues); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		if len(issues) == 0 {
			break
		}

		// Filter out pull requests (GitHub issues API includes PRs)
		for i := range issues {
			if issues[i].PullRequest == nil {
				allIssues = append(allIssues, issues[i])
			}
		}

		if len(issues) < perPage {
			break
		}

		page++
	}

	return allIssues, nil
}

// GitHubIssue represents an issue from GitHub API
type GitHubIssue struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`
	HTMLURL     string `json:"html_url"`
	User        struct {
		Login     string `json:"login"`
		ID        int64  `json:"id"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PullRequest *struct{}  `json:"pull_request,omitempty"` // Present if this is actually a PR
}

// GetAllComments fetches all issue comments with pagination
func (c *Client) GetAllComments(ctx context.Context, owner, repo string) ([]GitHubComment, error) {
	allComments := []GitHubComment{}
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/comments?per_page=%d&page=%d",
			owner, repo, perPage, page)

		resp, err := c.doRequest(ctx, "GET", url)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, readErrorAndClose(resp)
		}

		var comments []GitHubComment
		if err := readAndClose(resp, &comments); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		if len(comments) == 0 {
			break
		}

		allComments = append(allComments, comments...)

		if len(comments) < perPage {
			break
		}

		page++
	}

	return allComments, nil
}

// GitHubComment represents a comment from GitHub API
type GitHubComment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	User      struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"user"`
	IssueURL  string    `json:"issue_url"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetCommentReactions fetches all reactions for a comment with pagination.
// GitHub returns max 100 per page; this follows Link: rel="next" headers.
func (c *Client) GetCommentReactions(ctx context.Context, owner, repo string, commentID int64) ([]DetailedReaction, error) {
	return c.fetchAllReactions(ctx, fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/comments/%d/reactions?per_page=100", owner, repo, commentID))
}

// fetchAllReactions paginates through all reaction pages for a given URL.
// Closes response bodies immediately (not deferred) to prevent connection leaks.
func (c *Client) fetchAllReactions(ctx context.Context, firstURL string) ([]DetailedReaction, error) {
	var allReactions []DetailedReaction
	url := firstURL

	for page := 1; page <= 50; page++ { // Safety cap: 50 pages = 5,000 reactions
		resp, err := c.doRequest(ctx, "GET", url)
		if err != nil {
			return allReactions, err // Return partial results
		}

		if resp.StatusCode != http.StatusOK {
			return allReactions, readErrorAndClose(resp)
		}

		var reactions []DetailedReaction
		linkHeader := resp.Header.Get("Link")
		if err := readAndClose(resp, &reactions); err != nil {
			return allReactions, fmt.Errorf("failed to decode response: %w", err)
		}

		allReactions = append(allReactions, reactions...)

		// Follow pagination
		nextURL := parseLinkNext(linkHeader)
		if nextURL == "" || len(reactions) == 0 {
			break
		}
		url = nextURL
	}

	return allReactions, nil
}

// GetStargazersWithTimestamps fetches all stargazers with timestamps
func (c *Client) GetStargazersWithTimestamps(ctx context.Context, owner, repo string) ([]Stargazer, error) {
	allStargazers := []Stargazer{}
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/stargazers?per_page=%d&page=%d",
			owner, repo, perPage, page)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		if c.token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
		}
		// Special header to get timestamps
		req.Header.Set("Accept", "application/vnd.github.star+json")
		req.Header.Set("User-Agent", "OpenChaos-Token-Gov")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, readErrorAndClose(resp)
		}

		var stargazers []Stargazer
		if err := readAndClose(resp, &stargazers); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		if len(stargazers) == 0 {
			break
		}

		allStargazers = append(allStargazers, stargazers...)

		if len(stargazers) < perPage {
			break
		}

		page++
	}

	return allStargazers, nil
}

// Stargazer represents a stargazer with timestamp
type Stargazer struct {
	StarredAt time.Time `json:"starred_at"`
	User      struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"user"`
}

// GetForks fetches all forks with pagination
func (c *Client) GetForks(ctx context.Context, owner, repo string) ([]Fork, error) {
	allForks := []Fork{}
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/forks?per_page=%d&page=%d",
			owner, repo, perPage, page)

		resp, err := c.doRequest(ctx, "GET", url)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, readErrorAndClose(resp)
		}

		var forks []Fork
		if err := readAndClose(resp, &forks); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		if len(forks) == 0 {
			break
		}

		allForks = append(allForks, forks...)

		if len(forks) < perPage {
			break
		}

		page++
	}

	return allForks, nil
}

// Fork represents a fork from GitHub API
type Fork struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	FullName  string    `json:"full_name"`
	Owner     struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"owner"`
	CreatedAt time.Time `json:"created_at"`
}
