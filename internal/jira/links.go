package jira

import (
	"context"
	"fmt"
)

// GetIssueLinks returns all links on an issue.
func (c *Client) GetIssueLinks(ctx context.Context, issueKey string) ([]IssueLink, error) {
	issue, err := c.GetIssue(ctx, issueKey, []string{"issuelinks"})
	if err != nil {
		return nil, fmt.Errorf("getting issue links for '%s': %w", issueKey, err)
	}
	return issue.Fields.IssueLinks, nil
}

// GetLinkTypes returns the list of available issue link types.
func (c *Client) GetLinkTypes(ctx context.Context) ([]IssueLinkType, error) {
	path := c.ApiPath("issueLinkType")
	var resp IssueLinkTypesResponse
	if err := c.Get(ctx, path, nil, &resp); err != nil {
		return nil, fmt.Errorf("getting issue link types: %w", err)
	}
	return resp.IssueLinkTypes, nil
}

// CreateLink creates a relationship link between two issues.
func (c *Client) CreateLink(ctx context.Context, inwardKey, outwardKey, linkTypeName, comment string) error {
	path := c.ApiPath("issueLink")

	payload := map[string]any{
		"type": map[string]string{
			"name": linkTypeName,
		},
		"inwardIssue": map[string]string{
			"key": inwardKey,
		},
		"outwardIssue": map[string]string{
			"key": outwardKey,
		},
	}

	if comment != "" {
		if c.apiVersion == "3" {
			payload["comment"] = map[string]any{
				"body": map[string]any{
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
				},
			}
		} else {
			payload["comment"] = map[string]any{
				"body": comment,
			}
		}
	}

	if err := c.Post(ctx, path, payload, nil); err != nil {
		return fmt.Errorf("creating link '%s' between %s and %s: %w", linkTypeName, inwardKey, outwardKey, err)
	}

	return nil
}
