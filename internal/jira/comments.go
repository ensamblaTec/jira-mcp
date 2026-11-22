package jira

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// GetComments returns comments on an issue with optional limit (default 50, max 100).
func (c *Client) GetComments(ctx context.Context, issueKey string, limit ...int) ([]Comment, error) {
	maxResults := 50
	if len(limit) > 0 && limit[0] > 0 {
		maxResults = limit[0]
		if maxResults > 100 {
			maxResults = 100
		}
	}
	path := c.ApiPath(fmt.Sprintf("issue/%s/comment", issueKey))
	q := url.Values{}
	q.Set("maxResults", strconv.Itoa(maxResults))
	q.Set("orderBy", "-created")

	var resp CommentsResponse
	if err := c.Get(ctx, path, q, &resp); err != nil {
		return nil, fmt.Errorf("getting comments for issue '%s': %w", issueKey, err)
	}
	if len(resp.Comments) > maxResults {
		resp.Comments = resp.Comments[:maxResults]
	}
	return resp.Comments, nil
}

// AddComment adds a new comment to an issue.
func (c *Client) AddComment(ctx context.Context, issueKey, body string) (*Comment, error) {
	path := c.ApiPath(fmt.Sprintf("issue/%s/comment", issueKey))

	var payload map[string]any
	if c.apiVersion == "3" {
		payload = map[string]any{
			"body": TextToADF(body),
		}
	} else {
		payload = map[string]any{
			"body": body,
		}
	}

	var comment Comment
	if err := c.Post(ctx, path, payload, &comment); err != nil {
		return nil, fmt.Errorf("adding comment to issue '%s': %w", issueKey, err)
	}

	return &comment, nil
}

// UpdateComment modifies an existing comment.
func (c *Client) UpdateComment(ctx context.Context, issueKey, commentID, body string) (*Comment, error) {
	path := c.ApiPath(fmt.Sprintf("issue/%s/comment/%s", issueKey, commentID))

	var payload map[string]any
	if c.apiVersion == "3" {
		payload = map[string]any{
			"body": TextToADF(body),
		}
	} else {
		payload = map[string]any{
			"body": body,
		}
	}

	var comment Comment
	if err := c.Put(ctx, path, payload, &comment); err != nil {
		return nil, fmt.Errorf("updating comment %s on issue '%s': %w", commentID, issueKey, err)
	}

	return &comment, nil
}

// DeleteComment removes a comment from an issue.
func (c *Client) DeleteComment(ctx context.Context, issueKey, commentID string) error {
	path := c.ApiPath(fmt.Sprintf("issue/%s/comment/%s", issueKey, commentID))
	if err := c.Delete(ctx, path); err != nil {
		return fmt.Errorf("deleting comment %s on issue '%s': %w", commentID, issueKey, err)
	}
	return nil
}
