package github

import (
	"sync"
	"time"
)

// PR represents a GitHub Pull Request
type PR struct {
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	State        string    `json:"state"`
	Author       string    `json:"author"`
	AuthorAvatar string    `json:"authorAvatar"`
	URL          string    `json:"url"`
	CreatedAt    string    `json:"createdAt"`
	UpdatedAt    string    `json:"updatedAt"`
	Merged       bool      `json:"merged"`
	CachedAt     time.Time `json:"-"`
}

// PRCache stores PR data in memory with automatic expiration
type PRCache struct {
	mu    sync.RWMutex
	prs   map[int]*PR  // number → PR
	ttl   time.Duration // Time-to-live for cached data
}

// NewPRCache creates a new PR cache
func NewPRCache(ttl time.Duration) *PRCache {
	if ttl == 0 {
		ttl = 5 * time.Minute // Default 5 minutes
	}

	return &PRCache{
		prs: make(map[int]*PR),
		ttl: ttl,
	}
}

// UpdatePR adds or updates a PR in the cache
func (c *PRCache) UpdatePR(pr *PR) {
	c.mu.Lock()
	defer c.mu.Unlock()

	pr.CachedAt = time.Now()
	c.prs[pr.Number] = pr
}

// GetPR retrieves a PR from cache
func (c *PRCache) GetPR(number int) (*PR, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	pr, exists := c.prs[number]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Since(pr.CachedAt) > c.ttl {
		return nil, false
	}

	return pr, true
}

// GetOpenPRs returns all open PRs from cache
func (c *PRCache) GetOpenPRs() []*PR {
	c.mu.RLock()
	defer c.mu.RUnlock()

	openPRs := make([]*PR, 0)
	for _, pr := range c.prs {
		if pr.State == "open" && time.Since(pr.CachedAt) <= c.ttl {
			openPRs = append(openPRs, pr)
		}
	}

	return openPRs
}

// GetAllPRs returns all PRs from cache (including closed)
func (c *PRCache) GetAllPRs() []*PR {
	c.mu.RLock()
	defer c.mu.RUnlock()

	allPRs := make([]*PR, 0, len(c.prs))
	for _, pr := range c.prs {
		if time.Since(pr.CachedAt) <= c.ttl {
			allPRs = append(allPRs, pr)
		}
	}

	return allPRs
}

// DeletePR removes a PR from cache
func (c *PRCache) DeletePR(number int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.prs, number)
}

// Clear removes all PRs from cache
func (c *PRCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.prs = make(map[int]*PR)
}

// CleanExpired removes expired PRs from cache
func (c *PRCache) CleanExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	for number, pr := range c.prs {
		if time.Since(pr.CachedAt) > c.ttl {
			delete(c.prs, number)
			removed++
		}
	}

	return removed
}

// Count returns the number of cached PRs
func (c *PRCache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.prs)
}
