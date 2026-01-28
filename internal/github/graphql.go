package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GraphQLClient handles GitHub GraphQL API requests
type GraphQLClient struct {
	token      string
	httpClient *http.Client
}

// NewGraphQLClient creates a new GraphQL client
func NewGraphQLClient(token string) *GraphQLClient {
	return &GraphQLClient{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GraphQLRequest represents a GraphQL request
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL response
type GraphQLResponse struct {
	Data   json.RawMessage        `json:"data"`
	Errors []GraphQLError         `json:"errors,omitempty"`
}

// GraphQLError represents a GraphQL error
type GraphQLError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// doQuery executes a GraphQL query
func (c *GraphQLClient) doQuery(ctx context.Context, query string, variables map[string]interface{}) (*GraphQLResponse, error) {
	reqBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "OpenChaos-Token-Gov")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github GraphQL error %d: %s", resp.StatusCode, string(body))
	}

	var gqlResp GraphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("graphql errors: %v", gqlResp.Errors)
	}

	return &gqlResp, nil
}

// Discussion represents a GitHub discussion
type Discussion struct {
	Number    int                 `json:"number"`
	Title     string              `json:"title"`
	Author    DiscussionAuthor    `json:"author"`
	CreatedAt time.Time           `json:"createdAt"`
	UpdatedAt time.Time           `json:"updatedAt"`
	Comments  []DiscussionComment `json:"comments"`
	Reactions []DiscussionReaction `json:"reactions"`
}

// DiscussionAuthor represents a discussion author
type DiscussionAuthor struct {
	Login string `json:"login"`
}

// DiscussionComment represents a discussion comment
type DiscussionComment struct {
	Number    int              `json:"number"`
	Body      string           `json:"body"`
	Author    DiscussionAuthor `json:"author"`
	CreatedAt time.Time        `json:"createdAt"`
	IsAnswer  bool             `json:"isAnswer"`
}

// DiscussionReaction represents a reaction on a discussion
type DiscussionReaction struct {
	Number    int              `json:"number"`
	Content   string           `json:"content"`
	User      DiscussionAuthor `json:"user"`
	CreatedAt time.Time        `json:"createdAt"`
}

// FetchDiscussions fetches discussions from a repository
func (c *GraphQLClient) FetchDiscussions(ctx context.Context, owner, repo string) ([]Discussion, error) {
	// GitHub GraphQL has a 500,000 node limit per query.
	// 25 discussions × 50 comments × 50 reactions = 62,500 nodes (well under limit).
	// Previous: 100 × 100 × 100 = 1,000,000 → MAX_NODE_LIMIT_EXCEEDED
	query := `
		query($owner: String!, $repo: String!, $first: Int!, $after: String) {
			repository(owner: $owner, name: $repo) {
				discussions(first: $first, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
					pageInfo {
						hasNextPage
						endCursor
					}
					nodes {
						number
						title
						author {
							login
						}
						createdAt
						updatedAt
						reactions(first: 50) {
							nodes {
								content
								user {
									login
								}
								createdAt
							}
						}
						comments(first: 50) {
							nodes {
								body
								author {
									login
								}
								createdAt
								isAnswer
								reactions(first: 50) {
									nodes {
										content
										user {
											login
										}
										createdAt
									}
								}
							}
						}
					}
				}
			}
		}
	`

	var allDiscussions []Discussion
	var cursor *string

	// Paginate through all discussions (25 per page, max 10 pages = 250 discussions)
	for page := 0; page < 10; page++ {
		variables := map[string]interface{}{
			"owner": owner,
			"repo":  repo,
			"first": 25,
		}
		if cursor != nil {
			variables["after"] = *cursor
		}

		resp, err := c.doQuery(ctx, query, variables)
		if err != nil {
			if len(allDiscussions) > 0 {
				return allDiscussions, nil // Return partial results
			}
			return nil, err
		}

		// Parse response
		var result struct {
			Repository struct {
				Discussions struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						Number    int       `json:"number"`
						Title     string    `json:"title"`
						Author    struct {
							Login string `json:"login"`
						} `json:"author"`
						CreatedAt time.Time `json:"createdAt"`
						UpdatedAt time.Time `json:"updatedAt"`
						Reactions struct {
							Nodes []struct {
								Content   string    `json:"content"`
								User      struct {
									Login string `json:"login"`
								} `json:"user"`
								CreatedAt time.Time `json:"createdAt"`
							} `json:"nodes"`
						} `json:"reactions"`
						Comments struct {
							Nodes []struct {
								Body      string    `json:"body"`
								Author    struct {
									Login string `json:"login"`
								} `json:"author"`
								CreatedAt time.Time `json:"createdAt"`
								IsAnswer  bool      `json:"isAnswer"`
								Reactions struct {
									Nodes []struct {
										Content   string    `json:"content"`
										User      struct {
											Login string `json:"login"`
										} `json:"user"`
										CreatedAt time.Time `json:"createdAt"`
									} `json:"nodes"`
								} `json:"reactions"`
							} `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"discussions"`
			} `json:"repository"`
		}

		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return allDiscussions, fmt.Errorf("failed to parse discussions: %w", err)
		}

		for _, node := range result.Repository.Discussions.Nodes {
			discussion := Discussion{
				Number: node.Number,
				Title:  node.Title,
				Author: DiscussionAuthor{
					Login: node.Author.Login,
				},
				CreatedAt: node.CreatedAt,
				UpdatedAt: node.UpdatedAt,
				Comments:  make([]DiscussionComment, 0, len(node.Comments.Nodes)),
				Reactions: make([]DiscussionReaction, 0, len(node.Reactions.Nodes)),
			}

			for i, commentNode := range node.Comments.Nodes {
				comment := DiscussionComment{
					Number: i + 1,
					Body:   commentNode.Body,
					Author: DiscussionAuthor{
						Login: commentNode.Author.Login,
					},
					CreatedAt: commentNode.CreatedAt,
					IsAnswer:  commentNode.IsAnswer,
				}
				discussion.Comments = append(discussion.Comments, comment)
			}

			for i, reactionNode := range node.Reactions.Nodes {
				reaction := DiscussionReaction{
					Number:  i + 1,
					Content: reactionNode.Content,
					User: DiscussionAuthor{
						Login: reactionNode.User.Login,
					},
					CreatedAt: reactionNode.CreatedAt,
				}
				discussion.Reactions = append(discussion.Reactions, reaction)
			}

			allDiscussions = append(allDiscussions, discussion)
		}

		if !result.Repository.Discussions.PageInfo.HasNextPage {
			break
		}
		endCursor := result.Repository.Discussions.PageInfo.EndCursor
		cursor = &endCursor
	}

	return allDiscussions, nil
}
