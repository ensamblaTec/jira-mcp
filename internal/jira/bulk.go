package jira

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// BulkCreateIssues creates multiple issues concurrently with controlled concurrency.
func (c *Client) BulkCreateIssues(ctx context.Context, requests []CreateIssueRequest) (*BulkOperationResult, error) {
	if len(requests) == 0 {
		return &BulkOperationResult{}, nil
	}

	result := &BulkOperationResult{
		TotalProcessed: len(requests),
		SuccessfulKeys: make([]string, 0, len(requests)),
		Errors:         make([]string, 0),
	}

	var mu sync.Mutex
	sem := make(chan struct{}, 5) // max 5 concurrent requests to avoid rate limits
	var wg sync.WaitGroup

	for i, req := range requests {
		select {
		case <-ctx.Done():
			mu.Lock()
			result.FailureCount++
			result.Errors = append(result.Errors, fmt.Sprintf("Item #%d ('%s'): %v", i+1, req.Summary, ctx.Err()))
			mu.Unlock()
			continue
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(idx int, r CreateIssueRequest) {
			defer wg.Done()
			defer func() { <-sem }()

			created, err := c.CreateIssue(ctx, r)
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.FailureCount++
				result.Errors = append(result.Errors, fmt.Sprintf("Item #%d ('%s'): %v", idx+1, r.Summary, err))
			} else {
				result.SuccessCount++
				result.SuccessfulKeys = append(result.SuccessfulKeys, created.Key)
			}
		}(i, req)
	}

	wg.Wait()
	return result, nil
}

// BulkUpdateIssues updates common fields across multiple issues.
func (c *Client) BulkUpdateIssues(ctx context.Context, issueKeys []string, req UpdateIssueRequest) (*BulkOperationResult, error) {
	if len(issueKeys) == 0 {
		return &BulkOperationResult{}, nil
	}

	result := &BulkOperationResult{
		TotalProcessed: len(issueKeys),
		SuccessfulKeys: make([]string, 0, len(issueKeys)),
		Errors:         make([]string, 0),
	}

	var mu sync.Mutex
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, key := range issueKeys {
		select {
		case <-ctx.Done():
			mu.Lock()
			result.FailureCount++
			result.Errors = append(result.Errors, fmt.Sprintf("Issue %s: %v", key, ctx.Err()))
			mu.Unlock()
			continue
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			defer func() { <-sem }()

			err := c.UpdateIssue(ctx, k, req)
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.FailureCount++
				result.Errors = append(result.Errors, fmt.Sprintf("Issue %s: %v", k, err))
			} else {
				result.SuccessCount++
				result.SuccessfulKeys = append(result.SuccessfulKeys, k)
			}
		}(key)
	}

	wg.Wait()
	return result, nil
}

// BulkTransitionIssues executes a transition (by name or ID) on multiple issues.
func (c *Client) BulkTransitionIssues(ctx context.Context, issueKeys []string, targetStateOrID string, comment string) (*BulkOperationResult, error) {
	if len(issueKeys) == 0 {
		return &BulkOperationResult{}, nil
	}

	result := &BulkOperationResult{
		TotalProcessed: len(issueKeys),
		SuccessfulKeys: make([]string, 0, len(issueKeys)),
		Errors:         make([]string, 0),
	}

	var mu sync.Mutex
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, key := range issueKeys {
		select {
		case <-ctx.Done():
			mu.Lock()
			result.FailureCount++
			result.Errors = append(result.Errors, fmt.Sprintf("Issue %s: %v", key, ctx.Err()))
			mu.Unlock()
			continue
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Resolve transition ID
			transID := targetStateOrID
			transitions, err := c.GetTransitions(ctx, k)
			if err == nil {
				targetLower := strings.ToLower(targetStateOrID)
				for _, t := range transitions {
					if t.ID == targetStateOrID || strings.ToLower(t.Name) == targetLower || (t.To != nil && strings.ToLower(t.To.Name) == targetLower) {
						transID = t.ID
						break
					}
				}
			}

			err = c.TransitionIssue(ctx, k, transID, comment)
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.FailureCount++
				result.Errors = append(result.Errors, fmt.Sprintf("Issue %s: %v", k, err))
			} else {
				result.SuccessCount++
				result.SuccessfulKeys = append(result.SuccessfulKeys, k)
			}
		}(key)
	}

	wg.Wait()
	return result, nil
}
