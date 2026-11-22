package jira

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ListAttachments returns all attachments on an issue.
func (c *Client) ListAttachments(ctx context.Context, issueKey string) ([]Attachment, error) {
	issue, err := c.GetIssue(ctx, issueKey, []string{"attachment"})
	if err != nil {
		return nil, fmt.Errorf("listing attachments for '%s': %w", issueKey, err)
	}
	return issue.Fields.Attachment, nil
}

// DownloadAttachment downloads an attachment by ID and writes it to destPath.
func (c *Client) DownloadAttachment(ctx context.Context, attachmentID string, destPath string) (*Attachment, error) {
	// First get attachment metadata
	metaPath := c.ApiPath(fmt.Sprintf("attachment/%s", attachmentID))
	var meta Attachment
	if err := c.Get(ctx, metaPath, nil, &meta); err != nil {
		return nil, fmt.Errorf("getting attachment metadata for %s: %w", attachmentID, err)
	}

	if meta.Content == "" {
		return nil, fmt.Errorf("attachment %s has no content download URL", attachmentID)
	}

	data, _, err := c.GetRaw(ctx, meta.Content)
	if err != nil {
		return nil, fmt.Errorf("downloading attachment %s content: %w", attachmentID, err)
	}

	// If destPath is a directory, append the original filename
	info, err := os.Stat(destPath)
	targetFile := destPath
	if err == nil && info.IsDir() {
		targetFile = filepath.Join(destPath, meta.Filename)
	} else if err != nil && os.IsNotExist(err) && (destPath[len(destPath)-1] == '/' || destPath[len(destPath)-1] == filepath.Separator) {
		if err := os.MkdirAll(destPath, 0755); err != nil {
			return nil, fmt.Errorf("creating target directory: %w", err)
		}
		targetFile = filepath.Join(destPath, meta.Filename)
	}

	// Ensure parent dir exists
	parentDir := filepath.Dir(targetFile)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("creating parent directory '%s': %w", parentDir, err)
	}

	if err := os.WriteFile(targetFile, data, 0644); err != nil {
		return nil, fmt.Errorf("saving attachment to '%s': %w", targetFile, err)
	}

	meta.Content = targetFile // Return local path in metadata content field
	return &meta, nil
}

// UploadAttachment uploads a local file and attaches it to an issue.
func (c *Client) UploadAttachment(ctx context.Context, issueKey string, localFilePath string) ([]Attachment, error) {
	fileData, err := os.ReadFile(localFilePath)
	if err != nil {
		return nil, fmt.Errorf("reading file '%s' to upload: %w", localFilePath, err)
	}

	filename := filepath.Base(localFilePath)
	path := c.ApiPath(fmt.Sprintf("issue/%s/attachments", issueKey))

	var created []Attachment
	if err := c.PostMultipart(ctx, path, filename, fileData, &created); err != nil {
		return nil, fmt.Errorf("uploading attachment '%s' to issue '%s': %w", filename, issueKey, err)
	}

	return created, nil
}
