package jira

import (
	"context"
	"fmt"
	"time"
)

// GetWorklogs returns worklogs for an issue.
func (c *Client) GetWorklogs(ctx context.Context, issueKey string) ([]Worklog, error) {
	path := c.ApiPath(fmt.Sprintf("issue/%s/worklog", issueKey))
	var resp WorklogsResponse
	if err := c.Get(ctx, path, nil, &resp); err != nil {
		return nil, fmt.Errorf("getting worklogs for issue '%s': %w", issueKey, err)
	}
	return resp.Worklogs, nil
}

// AddWorklog logs work/time on an issue.
func (c *Client) AddWorklog(ctx context.Context, issueKey string, timeSpent string, comment string, started string) (*Worklog, error) {
	path := c.ApiPath(fmt.Sprintf("issue/%s/worklog", issueKey))

	if started == "" {
		started = time.Now().Format("2006-01-02T15:04:05.000-0700")
	}

	payload := map[string]any{
		"timeSpent": timeSpent,
		"started":   started,
	}

	if comment != "" {
		if c.apiVersion == "3" {
			payload["comment"] = map[string]any{
				"version": 1,
				"type":    "doc",
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{
								"type": "text",
								"text": comment,
							},
						},
					},
				},
			}
		} else {
			payload["comment"] = comment
		}
	}

	var worklog Worklog
	if err := c.Post(ctx, path, payload, &worklog); err != nil {
		return nil, fmt.Errorf("adding worklog to issue '%s': %w", issueKey, err)
	}

	return &worklog, nil
}
