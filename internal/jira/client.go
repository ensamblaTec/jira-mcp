package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ensamblatec/jira-mcp/internal/config"
)

// Client handles HTTP communication with the Jira REST API.
type Client struct {
	baseURL      string
	authHeader   string
	apiVersion   string
	httpClient   *http.Client
	debug        bool
	myselfMu     sync.RWMutex
	cachedMyself *User
}

// JiraErrorResponse models Jira's standard error JSON body.
type JiraErrorResponse struct {
	ErrorMessages []string          `json:"errorMessages,omitempty"`
	Errors        map[string]string `json:"errors,omitempty"`
	Message       string            `json:"message,omitempty"`
}

// NewClient creates a new Jira API client from the configuration.
func NewClient(cfg *config.Config) *Client {
	var authHeader string
	if cfg.AuthType == "basic" {
		authStr := fmt.Sprintf("%s:%s", cfg.Email, cfg.APIToken)
		authHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(authStr))
	} else {
		authHeader = "Bearer " + cfg.APIToken
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		authHeader: authHeader,
		apiVersion: cfg.APIVersion,
		httpClient: &http.Client{Timeout: timeout},
		debug:      cfg.Debug,
	}
}

// ResolveMyself returns the authenticated user info cached in memory for the lifetime of the client.
func (c *Client) ResolveMyself(ctx context.Context) (*User, error) {
	c.myselfMu.RLock()
	if c.cachedMyself != nil {
		user := c.cachedMyself
		c.myselfMu.RUnlock()
		return user, nil
	}
	c.myselfMu.RUnlock()

	c.myselfMu.Lock()
	defer c.myselfMu.Unlock()
	// Double-check after acquiring write lock
	if c.cachedMyself != nil {
		return c.cachedMyself, nil
	}

	user, err := c.GetMyself(ctx)
	if err != nil {
		return nil, err
	}
	c.cachedMyself = user
	return user, nil
}

// ResolveAssignee resolves alias "@me", "me", or "myself" into the authenticated user's accountId or username.
func (c *Client) ResolveAssignee(ctx context.Context, assignee string) (string, error) {
	assignee = strings.TrimSpace(assignee)
	if strings.EqualFold(assignee, "@me") || strings.EqualFold(assignee, "me") || strings.EqualFold(assignee, "myself") {
		user, err := c.ResolveMyself(ctx)
		if err != nil {
			return "", fmt.Errorf("resolving '@me' assignee: %w", err)
		}
		if user.AccountID != "" {
			return user.AccountID, nil
		}
		if user.Name != "" {
			return user.Name, nil
		}
		return user.DisplayName, nil
	}
	return assignee, nil
}

// ApiPath builds the API path according to configured API version (e.g. /rest/api/2/issue).
func (c *Client) ApiPath(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "/")
	return fmt.Sprintf("/rest/api/%s/%s", c.apiVersion, endpoint)
}

// AgilePath builds the Jira Software Agile API path (e.g. /rest/agile/1.0/board).
func (c *Client) AgilePath(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "/")
	return fmt.Sprintf("/rest/agile/1.0/%s", endpoint)
}

func parseRetryAfter(header string, defaultDuration time.Duration) time.Duration {
	if header == "" {
		return defaultDuration
	}
	header = strings.TrimSpace(header)
	if secs, err := strconv.Atoi(header); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return defaultDuration
}

func (c *Client) executeWithRetry(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("User-Agent", "jira-mcp/1.0")

	maxRetries := 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, nil, fmt.Errorf("resetting request body for retry: %w", err)
			}
			req.Body = body
		}

		resp, err := c.httpClient.Do(req.WithContext(ctx))
		if err != nil {
			return nil, nil, fmt.Errorf("http request failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			_ = resp.Body.Close()
			baseWait := time.Duration(1<<attempt) * time.Second
			wait := parseRetryAfter(resp.Header.Get("Retry-After"), baseWait)
			if wait > 60*time.Second {
				wait = 60 * time.Second
			}
			if wait < 10*time.Millisecond {
				wait = 10 * time.Millisecond
			}

			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(wait):
				continue
			}
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("reading response body: %w", err)
		}

		return resp, bodyBytes, nil
	}

	return nil, nil, fmt.Errorf("exceeded max retries for request")
}

func (c *Client) doRequest(ctx context.Context, req *http.Request, out any) error {
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	resp, bodyBytes, err := c.executeWithRetry(ctx, req)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.parseErrorResponse(resp.StatusCode, bodyBytes)
	}

	if out != nil && len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, out); err != nil {
			return fmt.Errorf("unmarshaling json response: %w", err)
		}
	}

	return nil
}

func (c *Client) parseErrorResponse(statusCode int, body []byte) error {
	var jiraErr JiraErrorResponse
	if err := json.Unmarshal(body, &jiraErr); err == nil {
		var parts []string
		if len(jiraErr.ErrorMessages) > 0 {
			parts = append(parts, strings.Join(jiraErr.ErrorMessages, "; "))
		}
		for k, v := range jiraErr.Errors {
			parts = append(parts, fmt.Sprintf("%s: %s", k, v))
		}
		if jiraErr.Message != "" {
			parts = append(parts, jiraErr.Message)
		}
		if len(parts) > 0 {
			return fmt.Errorf("jira api error (%d): %s", statusCode, strings.Join(parts, " | "))
		}
	}

	trimmedBody := strings.TrimSpace(string(body))
	if len(trimmedBody) > 200 {
		trimmedBody = trimmedBody[:200] + "..."
	}
	return fmt.Errorf("jira api error (%d): %s", statusCode, trimmedBody)
}

// Get performs a GET request to the specified path with query parameters.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	reqURL := c.baseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}

	return c.doRequest(ctx, req, out)
}

// Post performs a POST request with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doRequest(ctx, req, out)
}

// Put performs a PUT request with a JSON body.
func (c *Client) Put(ctx context.Context, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doRequest(ctx, req, out)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}

	return c.doRequest(ctx, req, nil)
}

// PostMultipart uploads a file using multipart/form-data.
func (c *Client) PostMultipart(ctx context.Context, path string, filename string, fileBytes []byte, out any) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		return fmt.Errorf("writing file bytes: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body.Bytes()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Atlassian-Token", "no-check") // Required for Jira attachments

	return c.doRequest(ctx, req, out)
}

// GetRaw downloads raw bytes from a full URL or relative path.
func (c *Client) GetRaw(ctx context.Context, rawURL string) ([]byte, string, error) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = c.baseURL + rawURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, bodyBytes, err := c.executeWithRetry(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("download request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", c.parseErrorResponse(resp.StatusCode, bodyBytes)
	}

	contentType := resp.Header.Get("Content-Type")
	return bodyBytes, contentType, nil
}
