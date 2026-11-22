package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ensamblatec/jira-mcp/internal/config"
)

func setupMockJiraServer(t *testing.T) (*httptest.Server, *Client) {
	mux := http.NewServeMux()

	// Myself
	mux.HandleFunc("/rest/api/2/myself", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(User{
			AccountID:    "acc-123",
			EmailAddress: "user@example.com",
			DisplayName:  "Test Engineer",
			Active:       true,
		})
	})

	// Projects
	mux.HandleFunc("/rest/api/2/project", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Project{
			{ID: "1000", Key: "TEST", Name: "Test Project"},
		})
	})

	mux.HandleFunc("/rest/api/2/project/TEST", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Project{
			ID:   "1000",
			Key:  "TEST",
			Name: "Test Project",
		})
	})

	// Issues
	mux.HandleFunc("/rest/api/2/issue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(Issue{
				ID:  "1001",
				Key: "TEST-1",
			})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/rest/api/2/issue/TEST-1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(Issue{
				ID:  "1001",
				Key: "TEST-1",
				Fields: IssueFields{
					Summary:  "Test Summary",
					Status:   &Status{Name: "To Do", StatusCategory: &StatusCategory{Key: "new"}},
					Priority: &Priority{Name: "High"},
					Assignee: &User{DisplayName: "Alice"},
					Created:  "2026-09-01T10:00:00.000+0000",
					Updated:  "2026-09-20T10:00:00.000+0000",
				},
			})
		case http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Search
	mux.HandleFunc("/rest/api/2/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(SearchResult{
			Total:      1,
			StartAt:    0,
			MaxResults: 50,
			Issues: []Issue{
				{
					ID:  "1001",
					Key: "TEST-1",
					Fields: IssueFields{
						Summary:  "Test Summary",
						Status:   &Status{Name: "To Do", StatusCategory: &StatusCategory{Key: "new"}},
						Priority: &Priority{Name: "High"},
						Assignee: &User{DisplayName: "Alice"},
						Created:  "2026-08-01T10:00:00.000+0000", // > 30 days old
						Updated:  "2026-08-01T10:00:00.000+0000",
					},
				},
			},
		})
	})

	// Transitions
	mux.HandleFunc("/rest/api/2/issue/TEST-1/transitions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(TransitionsResponse{
				Transitions: []Transition{
					{ID: "21", Name: "In Progress", To: &Status{Name: "In Progress"}},
					{ID: "31", Name: "Done", To: &Status{Name: "Done"}},
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	// Agile Boards
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(BoardsResponse{
			Total: 1,
			Values: []Board{
				{ID: 10, Name: "Test Board", Type: "scrum"},
			},
		})
	})

	// Agile Board Configuration
	mux.HandleFunc("/rest/agile/1.0/board/10/configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(BoardConfiguration{
			ID:   10,
			Name: "Test Board",
			Type: "scrum",
			Location: &BoardLocation{
				ProjectKey: "TEST",
			},
			ColumnConfig: ColumnConfig{
				Columns: []BoardColumn{
					{Name: "To Do", Statuses: []BoardColumnStatus{{ID: "1"}}},
					{Name: "Done", Statuses: []BoardColumnStatus{{ID: "3"}}},
				},
			},
		})
	})

	// Agile Board Issues
	mux.HandleFunc("/rest/agile/1.0/board/10/issue", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(SearchResult{
			Total: 1,
			Issues: []Issue{
				{
					ID:  "1001",
					Key: "TEST-1",
					Fields: IssueFields{
						Summary:  "Test Summary",
						Status:   &Status{Name: "Done", StatusCategory: &StatusCategory{Key: "done"}},
						Priority: &Priority{Name: "High"},
						Assignee: &User{DisplayName: "Alice"},
					},
				},
			},
		})
	})

	// Agile Sprints
	mux.HandleFunc("/rest/agile/1.0/board/10/sprint", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(SprintsResponse{
			Total: 1,
			Values: []Sprint{
				{ID: 100, Name: "Sprint 1", State: "active", OriginBoardID: 10},
			},
		})
	})

	mux.HandleFunc("/rest/agile/1.0/sprint/100", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Sprint{
			ID:            100,
			Name:          "Sprint 1",
			State:         "active",
			OriginBoardID: 10,
		})
	})

	mux.HandleFunc("/rest/agile/1.0/sprint/100/issue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(SearchResult{
				Total: 1,
				Issues: []Issue{
					{
						ID:  "1001",
						Key: "TEST-1",
						Fields: IssueFields{
							Summary:  "Test Summary",
							Status:   &Status{Name: "Done", StatusCategory: &StatusCategory{Key: "done"}},
							Priority: &Priority{Name: "High"},
							Assignee: &User{DisplayName: "Alice"},
						},
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/rest/agile/1.0/board/10/backlog", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(SearchResult{
			Total: 1,
			Issues: []Issue{
				{
					ID:  "1002",
					Key: "TEST-2",
					Fields: IssueFields{
						Summary:  "Backlog item",
						Status:   &Status{Name: "To Do"},
						Priority: &Priority{Name: "Medium"},
						Created:  "2026-08-01T10:00:00.000+0000",
						Updated:  "2026-08-01T10:00:00.000+0000",
					},
				},
			},
		})
	})

	// Comments
	mux.HandleFunc("/rest/api/2/issue/TEST-1/comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(CommentsResponse{
				Total: 1,
				Comments: []Comment{
					{ID: "501", Body: "Hello Jira!"},
				},
			})
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(Comment{ID: "502", Body: "New Comment"})
			return
		}
	})

	// Worklog
	mux.HandleFunc("/rest/api/2/issue/TEST-1/worklog", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(WorklogsResponse{
				Total: 1,
				Worklogs: []Worklog{
					{ID: "601", TimeSpent: "2h"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Worklog{ID: "602", TimeSpent: "1h 30m"})
	})

	// Attachments
	mux.HandleFunc("/rest/api/2/attachment/701", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Attachment{
			ID:       "701",
			Filename: "report.pdf",
			Content:  "/download/report.pdf",
		})
	})

	mux.HandleFunc("/download/report.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.4 dummy pdf content"))
	})

	mux.HandleFunc("/rest/api/2/issue/TEST-1/attachments", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]Attachment{
			{ID: "702", Filename: "uploaded.txt"},
		})
	})

	srv := httptest.NewServer(mux)

	cfg := &config.Config{
		BaseURL:    srv.URL,
		Email:      "user@example.com",
		APIToken:   "test-token",
		AuthType:   "basic",
		APIVersion: "2",
	}

	client := NewClient(cfg)
	return srv, client
}

func TestJiraClient_MyselfAndProjects(t *testing.T) {
	srv, client := setupMockJiraServer(t)
	defer srv.Close()

	ctx := context.Background()

	// Myself
	user, err := client.GetMyself(ctx)
	if err != nil {
		t.Fatalf("GetMyself failed: %v", err)
	}
	if user.DisplayName != "Test Engineer" {
		t.Errorf("expected 'Test Engineer', got '%s'", user.DisplayName)
	}

	// Projects
	projects, err := client.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(projects) != 1 || projects[0].Key != "TEST" {
		t.Errorf("expected project 'TEST', got %+v", projects)
	}

	proj, err := client.GetProject(ctx, "TEST")
	if err != nil {
		t.Fatalf("GetProject failed: %v", err)
	}
	if proj.Key != "TEST" {
		t.Errorf("expected project 'TEST', got '%s'", proj.Key)
	}
}

func TestJiraClient_IssuesAndTransitions(t *testing.T) {
	srv, client := setupMockJiraServer(t)
	defer srv.Close()

	ctx := context.Background()

	// Create
	created, err := client.CreateIssue(ctx, CreateIssueRequest{
		ProjectKey: "TEST",
		IssueType:  "Story",
		Summary:    "Create story test",
	})
	if err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}
	if created.Key != "TEST-1" {
		t.Errorf("expected TEST-1, got '%s'", created.Key)
	}

	// Get
	issue, err := client.GetIssue(ctx, "TEST-1", nil)
	if err != nil {
		t.Fatalf("GetIssue failed: %v", err)
	}
	if issue.Fields.Summary != "Test Summary" {
		t.Errorf("expected 'Test Summary', got '%s'", issue.Fields.Summary)
	}

	// Search
	searchRes, err := client.SearchIssues(ctx, "project = TEST", nil, 0, 10)
	if err != nil {
		t.Fatalf("SearchIssues failed: %v", err)
	}
	if len(searchRes.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(searchRes.Issues))
	}

	// Update
	if err := client.UpdateIssue(ctx, "TEST-1", UpdateIssueRequest{Summary: "Updated Summary"}); err != nil {
		t.Fatalf("UpdateIssue failed: %v", err)
	}

	// Transitions
	transitions, err := client.GetTransitions(ctx, "TEST-1")
	if err != nil {
		t.Fatalf("GetTransitions failed: %v", err)
	}
	if len(transitions) != 2 {
		t.Errorf("expected 2 transitions, got %d", len(transitions))
	}

	if err := client.TransitionIssue(ctx, "TEST-1", "21", "Moving to in progress"); err != nil {
		t.Fatalf("TransitionIssue failed: %v", err)
	}

	// Delete
	if err := client.DeleteIssue(ctx, "TEST-1"); err != nil {
		t.Fatalf("DeleteIssue failed: %v", err)
	}
}

func TestJiraClient_Agile(t *testing.T) {
	srv, client := setupMockJiraServer(t)
	defer srv.Close()

	ctx := context.Background()

	boards, err := client.ListBoards(ctx, "scrum", 0, 10)
	if err != nil {
		t.Fatalf("ListBoards failed: %v", err)
	}
	if len(boards) != 1 || boards[0].ID != 10 {
		t.Errorf("expected board 10, got %+v", boards)
	}

	sprints, err := client.ListSprints(ctx, 10, "active", 0, 10)
	if err != nil {
		t.Fatalf("ListSprints failed: %v", err)
	}
	if len(sprints) != 1 || sprints[0].ID != 100 {
		t.Errorf("expected sprint 100, got %+v", sprints)
	}

	sprintIssues, err := client.GetSprintIssues(ctx, 100, 0, 10)
	if err != nil {
		t.Fatalf("GetSprintIssues failed: %v", err)
	}
	if len(sprintIssues.Issues) != 1 {
		t.Errorf("expected 1 sprint issue, got %d", len(sprintIssues.Issues))
	}

	backlogIssues, err := client.GetBacklogIssues(ctx, 10, 0, 10)
	if err != nil {
		t.Fatalf("GetBacklogIssues failed: %v", err)
	}
	if len(backlogIssues.Issues) != 1 {
		t.Errorf("expected 1 backlog issue, got %d", len(backlogIssues.Issues))
	}

	// Board Configuration
	boardCfg, err := client.GetBoardConfiguration(ctx, 10)
	if err != nil {
		t.Fatalf("GetBoardConfiguration failed: %v", err)
	}
	if boardCfg.ID != 10 || len(boardCfg.ColumnConfig.Columns) != 2 {
		t.Errorf("unexpected board config: %+v", boardCfg)
	}

	// Build Board View
	view, err := client.BuildBoardView(ctx, 10, true)
	if err != nil {
		t.Fatalf("BuildBoardView failed: %v", err)
	}
	if view.BoardID != 10 || view.BoardType != "scrum" {
		t.Errorf("unexpected view properties: %+v", view)
	}
	if len(view.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(view.Columns))
	}
	if len(view.Backlog) != 1 {
		t.Errorf("expected 1 backlog card, got %d", len(view.Backlog))
	}
	markdown := view.ToMarkdown()
	if len(markdown) == 0 {
		t.Errorf("expected markdown representation, got empty")
	}
}

func TestJiraClient_CommentsWorklogAttachments(t *testing.T) {
	srv, client := setupMockJiraServer(t)
	defer srv.Close()

	ctx := context.Background()

	// Comments
	comments, err := client.GetComments(ctx, "TEST-1")
	if err != nil {
		t.Fatalf("GetComments failed: %v", err)
	}
	if len(comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(comments))
	}

	c, err := client.AddComment(ctx, "TEST-1", "A brand new comment")
	if err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}
	if c.ID != "502" {
		t.Errorf("expected comment ID 502, got %s", c.ID)
	}

	// Worklog
	worklogs, err := client.GetWorklogs(ctx, "TEST-1")
	if err != nil {
		t.Fatalf("GetWorklogs failed: %v", err)
	}
	if len(worklogs) != 1 {
		t.Errorf("expected 1 worklog, got %d", len(worklogs))
	}

	wl, err := client.AddWorklog(ctx, "TEST-1", "1h 30m", "Dev work", "")
	if err != nil {
		t.Fatalf("AddWorklog failed: %v", err)
	}
	if wl.ID != "602" {
		t.Errorf("expected worklog 602, got %s", wl.ID)
	}

	// Attachment Download
	tmpDir, err := os.MkdirTemp("", "jira-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	downloadPath := filepath.Join(tmpDir, "report.pdf")
	att, err := client.DownloadAttachment(ctx, "701", downloadPath)
	if err != nil {
		t.Fatalf("DownloadAttachment failed: %v", err)
	}
	if att.Filename != "report.pdf" {
		t.Errorf("expected report.pdf, got %s", att.Filename)
	}
	content, _ := os.ReadFile(downloadPath)
	if !strings.Contains(string(content), "%PDF") {
		t.Errorf("expected pdf content, got %s", string(content))
	}

	// Attachment Upload
	uploadFile := filepath.Join(tmpDir, "test-upload.txt")
	os.WriteFile(uploadFile, []byte("Hello world test"), 0644)
	uploaded, err := client.UploadAttachment(ctx, "TEST-1", uploadFile)
	if err != nil {
		t.Fatalf("UploadAttachment failed: %v", err)
	}
	if len(uploaded) != 1 {
		t.Errorf("expected 1 uploaded attachment, got %d", len(uploaded))
	}
}

func TestJiraClient_BulkAndAnalysis(t *testing.T) {
	srv, client := setupMockJiraServer(t)
	defer srv.Close()

	ctx := context.Background()

	// Bulk create
	bulkRes, err := client.BulkCreateIssues(ctx, []CreateIssueRequest{
		{ProjectKey: "TEST", IssueType: "Story", Summary: "Story 1"},
		{ProjectKey: "TEST", IssueType: "Story", Summary: "Story 2"},
	})
	if err != nil {
		t.Fatalf("BulkCreateIssues failed: %v", err)
	}
	if bulkRes.SuccessCount != 2 {
		t.Errorf("expected 2 successful creations, got %d", bulkRes.SuccessCount)
	}

	// Bulk transition
	bulkTrans, err := client.BulkTransitionIssues(ctx, []string{"TEST-1"}, "Done", "Bulk done")
	if err != nil {
		t.Fatalf("BulkTransitionIssues failed: %v", err)
	}
	if bulkTrans.SuccessCount != 1 {
		t.Errorf("expected 1 success transition, got %d", bulkTrans.SuccessCount)
	}

	// Backlog Analysis
	analysis, err := client.AnalyzeBacklog(ctx, 10, "")
	if err != nil {
		t.Fatalf("AnalyzeBacklog failed: %v", err)
	}
	if analysis.TotalIssues != 1 {
		t.Errorf("expected 1 backlog issue, got %d", analysis.TotalIssues)
	}
	summary := analysis.FormatAnalysisSummary()
	if !strings.Contains(summary, "Backlog Analysis Summary") {
		t.Errorf("expected summary markdown, got %s", summary)
	}

	// Sprint Analysis
	sprintAnalysis, err := client.AnalyzeSprint(ctx, 100)
	if err != nil {
		t.Fatalf("AnalyzeSprint failed: %v", err)
	}
	if sprintAnalysis.TotalIssues != 1 || sprintAnalysis.CompletedIssues != 1 {
		t.Errorf("expected 1 total and 1 completed, got %+v", sprintAnalysis)
	}

	// Export Issues (JSON, CSV, Markdown)
	tmpDir, _ := os.MkdirTemp("", "jira-export-*")
	defer os.RemoveAll(tmpDir)

	jsonOut := filepath.Join(tmpDir, "export.json")
	if _, err := client.ExportIssues(ctx, "project = TEST", "json", jsonOut); err != nil {
		t.Fatalf("ExportIssues json failed: %v", err)
	}

	csvOut := filepath.Join(tmpDir, "export.csv")
	if _, err := client.ExportIssues(ctx, "project = TEST", "csv", csvOut); err != nil {
		t.Fatalf("ExportIssues csv failed: %v", err)
	}

	mdOut := filepath.Join(tmpDir, "export.md")
	if _, err := client.ExportIssues(ctx, "project = TEST", "markdown", mdOut); err != nil {
		t.Fatalf("ExportIssues md failed: %v", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	defaultDur := 2 * time.Second

	// Empty header
	if d := parseRetryAfter("", defaultDur); d != defaultDur {
		t.Errorf("expected default %v, got %v", defaultDur, d)
	}

	// Integer seconds
	if d := parseRetryAfter("5", defaultDur); d != 5*time.Second {
		t.Errorf("expected 5s, got %v", d)
	}

	// Invalid string returns default
	if d := parseRetryAfter("invalid-seconds", defaultDur); d != defaultDur {
		t.Errorf("expected default %v, got %v", defaultDur, d)
	}

	// HTTP date in future
	future := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(future, defaultDur); d <= 0 || d > 12*time.Second {
		t.Errorf("expected future duration around 10s, got %v", d)
	}
}

func TestClient_RateLimitingRetry429(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att < 3 {
			// First 2 attempts return 429 with Retry-After: 0s
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"errorMessages":["Rate limit exceeded"]}`))
			return
		}
		// 3rd attempt succeeds
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Project{ID: "999", Key: "RETRY", Name: "Retry Project"})
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		BaseURL:    server.URL,
		APIToken:   "tok",
		APIVersion: "2",
	})

	ctx := context.Background()
	proj, err := client.GetProject(ctx, "RETRY")
	if err != nil {
		t.Fatalf("GetProject failed after retries: %v", err)
	}
	if proj.Key != "RETRY" {
		t.Errorf("expected RETRY project, got %s", proj.Key)
	}
	if attempts != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", attempts)
	}
}

func TestClient_RateLimitingExceeded(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"errorMessages":["Rate limit permanently exceeded"]}`))
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		BaseURL:    server.URL,
		APIToken:   "tok",
		APIVersion: "2",
	})

	ctx := context.Background()
	_, err := client.GetProject(ctx, "RETRY")
	if err == nil {
		t.Fatalf("expected error after exceeding retries, got nil")
	}
	if !strings.Contains(err.Error(), "Rate limit permanently exceeded") && !strings.Contains(err.Error(), "429") {
		t.Errorf("expected 429 error message, got: %v", err)
	}
	// Initial attempt + 3 retries = 4 total attempts
	if attempts != 4 {
		t.Errorf("expected 4 total attempts, got %d", attempts)
	}
}

func TestSanitizeLabel(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"frontend", "frontend"},
		{"backend api", "backend-api"},
		{"  auth service login  ", "auth-service-login"},
		{"", ""},
	}
	for _, c := range cases {
		if got := SanitizeLabel(c.in); got != c.want {
			t.Errorf("SanitizeLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGetIssueTemplate(t *testing.T) {
	storyTpl := GetIssueTemplate("story")
	if !strings.Contains(storyTpl, "Objetivo de Negocio") || !strings.Contains(storyTpl, "Definition of Done") {
		t.Errorf("unexpected story template: %s", storyTpl)
	}

	bugTpl := GetIssueTemplate("bug")
	if !strings.Contains(bugTpl, "Comportamiento Actual") || !strings.Contains(bugTpl, "Pasos para Reproducir") {
		t.Errorf("unexpected bug template: %s", bugTpl)
	}

	spikeTpl := GetIssueTemplate("spike")
	if !strings.Contains(spikeTpl, "Hipótesis") || !strings.Contains(spikeTpl, "Timebox") {
		t.Errorf("unexpected spike template: %s", spikeTpl)
	}

	if empty := GetIssueTemplate("unknown"); empty != "" {
		t.Errorf("expected empty string for unknown template, got %s", empty)
	}
}

func TestResolveAssignee_AtMe(t *testing.T) {
	var myselfCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/2/myself" {
			atomic.AddInt32(&myselfCalls, 1)
			json.NewEncoder(w).Encode(User{
				AccountID:   "acc-777",
				DisplayName: "Engineer Example",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		BaseURL:    server.URL,
		APIToken:   "tok",
		APIVersion: "2",
	})

	ctx := context.Background()

	// 1. Resolve "@me"
	res1, err := client.ResolveAssignee(ctx, "@me")
	if err != nil {
		t.Fatalf("ResolveAssignee(@me) failed: %v", err)
	}
	if res1 != "acc-777" {
		t.Errorf("expected acc-777, got %s", res1)
	}

	// 2. Resolve "me" (should use memory cache, no new HTTP call)
	res2, err := client.ResolveAssignee(ctx, "me")
	if err != nil {
		t.Fatalf("ResolveAssignee(me) failed: %v", err)
	}
	if res2 != "acc-777" {
		t.Errorf("expected acc-777, got %s", res2)
	}

	// 3. Resolve regular user string
	res3, err := client.ResolveAssignee(ctx, "other-user")
	if err != nil {
		t.Fatalf("ResolveAssignee(other) failed: %v", err)
	}
	if res3 != "other-user" {
		t.Errorf("expected other-user, got %s", res3)
	}

	// Verify only 1 HTTP call to /myself was made
	if myselfCalls != 1 {
		t.Errorf("expected exactly 1 call to /myself (memory cached), got %d", myselfCalls)
	}
}

func TestTransitionIssue_NameResolutionAndFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/issue/TEST-1/transitions":
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode(TransitionsResponse{
					Transitions: []Transition{
						{ID: "11", Name: "Start Progress", To: &Status{Name: "In Progress"}},
						{ID: "21", Name: "Review", To: &Status{Name: "In Review"}},
						{ID: "31", Name: "Deploy", To: &Status{Name: "DEPLOY"}},
					},
				})
				return
			}
			if r.Method == http.MethodPost {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				trans, _ := body["transition"].(map[string]any)
				if trans["id"] != "31" && trans["id"] != "21" {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"errorMessages":["Invalid transition ID"]}`))
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		BaseURL:    server.URL,
		APIToken:   "tok",
		APIVersion: "2",
	})

	ctx := context.Background()

	// 1. Direct numeric ID
	if err := client.TransitionIssue(ctx, "TEST-1", "21", "Moving to review"); err != nil {
		t.Fatalf("TransitionIssue with numeric ID failed: %v", err)
	}

	// 2. Resolve by status name (e.g. "DEPLOY" -> resolves to ID "31")
	if err := client.TransitionIssue(ctx, "TEST-1", "DEPLOY", "Ready to ship"); err != nil {
		t.Fatalf("TransitionIssue with status name failed: %v", err)
	}

	// 3. Case-insensitive transition name (e.g. "deploy" -> ID "31")
	if err := client.TransitionIssue(ctx, "TEST-1", "deploy", "lower case deploy"); err != nil {
		t.Fatalf("TransitionIssue with lowercase name failed: %v", err)
	}

	// 4. Non-existent transition name returns descriptive error with available transitions
	err := client.TransitionIssue(ctx, "TEST-1", "NonExistentState", "")
	if err == nil {
		t.Fatal("expected error for non-existent transition name")
	}
	if !strings.Contains(err.Error(), "available transitions") {
		t.Errorf("expected available transitions in error message, got: %v", err)
	}
}

func TestCreateIssue_TemplateAndSanitizedLabels(t *testing.T) {
	var capturedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/myself", "/rest/api/3/myself":
			json.NewEncoder(w).Encode(User{AccountID: "acc-auto-me"})
			return
		case "/rest/api/2/issue", "/rest/api/3/issue":
			json.NewDecoder(r.Body).Decode(&capturedPayload)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(Issue{ID: "1002", Key: "TEST-2"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		BaseURL:    server.URL,
		APIToken:   "tok",
		APIVersion: "3",
	})

	ctx := context.Background()
	_, err := client.CreateIssue(ctx, CreateIssueRequest{
		ProjectKey: "TEST",
		IssueType:  "Story",
		Summary:    "Template Verification",
		Template:   "story",
		Assignee:   "@me",
		Labels:     []string{"ui component", "auth service"},
	})
	if err != nil {
		t.Fatalf("CreateIssue failed: %v", err)
	}

	fields, ok := capturedPayload["fields"].(map[string]any)
	if !ok {
		t.Fatalf("expected fields map in payload: %+v", capturedPayload)
	}

	// Verify template was applied
	descBytes, _ := json.Marshal(fields["description"])
	if !strings.Contains(string(descBytes), "Definition of Done") {
		t.Errorf("expected Definition of Done in description, got: %s", string(descBytes))
	}

	// Verify @me resolved to acc-auto-me
	assigneeMap, _ := fields["assignee"].(map[string]any)
	if assigneeMap["accountId"] != "acc-auto-me" {
		t.Errorf("expected acc-auto-me assignee, got: %+v", assigneeMap)
	}

	// Verify labels were sanitized (spaces replaced with hyphens)
	labels, _ := fields["labels"].([]any)
	expectedLabels := map[string]bool{"ui-component": true, "auth-service": true}
	for _, l := range labels {
		lStr, _ := l.(string)
		if !expectedLabels[lStr] {
			t.Errorf("unexpected sanitized label: %s", lStr)
		}
	}
}

func TestUpdateIssue_AtomicLabels(t *testing.T) {
	var capturedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/myself", "/rest/api/3/myself":
			json.NewEncoder(w).Encode(User{AccountID: "acc-auto-me"})
			return
		case "/rest/api/2/issue/TEST-1", "/rest/api/3/issue/TEST-1":
			json.NewDecoder(r.Body).Decode(&capturedPayload)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		BaseURL:    server.URL,
		APIToken:   "tok",
		APIVersion: "3",
	})

	ctx := context.Background()
	err := client.UpdateIssue(ctx, "TEST-1", UpdateIssueRequest{
		AddLabels:    []string{"hotfix patch", "ready"},
		RemoveLabels: []string{"wip"},
		Assignee:     "@me",
	})
	if err != nil {
		t.Fatalf("UpdateIssue failed: %v", err)
	}

	// Verify update.labels atomic ops
	updateMap, ok := capturedPayload["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected update map in payload: %+v", capturedPayload)
	}
	labelOps, ok := updateMap["labels"].([]any)
	if !ok {
		t.Fatalf("expected labels array in update map: %+v", updateMap)
	}
	if len(labelOps) != 3 {
		t.Fatalf("expected 3 label operations (2 add, 1 remove), got %d: %+v", len(labelOps), labelOps)
	}

	// Verify assignee was resolved in fields
	fieldsMap, ok := capturedPayload["fields"].(map[string]any)
	if !ok {
		t.Fatalf("expected fields map in payload: %+v", capturedPayload)
	}
	assignee, _ := fieldsMap["assignee"].(map[string]any)
	if assignee["accountId"] != "acc-auto-me" {
		t.Errorf("expected acc-auto-me assignee, got: %+v", assignee)
	}
}

func TestIssueRequestUnmarshalSnakeAndCamel(t *testing.T) {
	t.Run("CreateIssueRequest camelCase", func(t *testing.T) {
		payload := `{
			"projectKey": "PROJ",
			"issueType": "Story",
			"summary": "Camel summary",
			"parentKey": "PROJ-10",
			"dueDate": "2026-12-31",
			"fixVersions": ["1.0.0"],
			"customFields": {"customfield_1001": "alpha"}
		}`
		var req CreateIssueRequest
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if req.ProjectKey != "PROJ" || req.IssueType != "Story" || req.ParentKey != "PROJ-10" || req.DueDate != "2026-12-31" {
			t.Errorf("unexpected scalar fields: %+v", req)
		}
		if !reflect.DeepEqual(req.FixVersions, []string{"1.0.0"}) {
			t.Errorf("unexpected FixVersions: %v", req.FixVersions)
		}
		if req.CustomFields["customfield_1001"] != "alpha" {
			t.Errorf("unexpected CustomFields: %v", req.CustomFields)
		}
	})

	t.Run("CreateIssueRequest snake_case", func(t *testing.T) {
		payload := `{
			"project_key": "TEST",
			"issue_type": "Bug",
			"summary": "Snake summary",
			"parent_key": "TEST-1",
			"due_date": "2026-11-30",
			"fix_versions": ["2.0.0", "2.1.0"],
			"custom_fields": {"customfield_1002": "beta"}
		}`
		var req CreateIssueRequest
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if req.ProjectKey != "TEST" || req.IssueType != "Bug" || req.ParentKey != "TEST-1" || req.DueDate != "2026-11-30" {
			t.Errorf("unexpected scalar fields: %+v", req)
		}
		if !reflect.DeepEqual(req.FixVersions, []string{"2.0.0", "2.1.0"}) {
			t.Errorf("unexpected FixVersions: %v", req.FixVersions)
		}
		if req.CustomFields["customfield_1002"] != "beta" {
			t.Errorf("unexpected CustomFields: %v", req.CustomFields)
		}
	})

	t.Run("CreateIssueRequest invalid JSON", func(t *testing.T) {
		var req CreateIssueRequest
		if err := json.Unmarshal([]byte(`{"project_key": 123`), &req); err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("UpdateIssueRequest camelCase and snake_case", func(t *testing.T) {
		camel := `{
			"addLabels": ["backend"],
			"removeLabels": ["stale"],
			"fixVersions": ["1.2.0"],
			"dueDate": "2026-12-01",
			"customFields": {"customfield_2001": 42}
		}`
		var reqCamel UpdateIssueRequest
		if err := json.Unmarshal([]byte(camel), &reqCamel); err != nil {
			t.Fatalf("camelCase Unmarshal failed: %v", err)
		}
		if !reflect.DeepEqual(reqCamel.AddLabels, []string{"backend"}) || !reflect.DeepEqual(reqCamel.RemoveLabels, []string{"stale"}) {
			t.Errorf("unexpected camelCase labels: %+v", reqCamel)
		}
		if !reflect.DeepEqual(reqCamel.FixVersions, []string{"1.2.0"}) || reqCamel.DueDate != "2026-12-01" {
			t.Errorf("unexpected camelCase fields: %+v", reqCamel)
		}

		snake := `{
			"add_labels": ["urgent", "api"],
			"remove_labels": ["wontfix"],
			"fix_versions": ["3.0.0"],
			"due_date": "2026-12-15",
			"custom_fields": {"customfield_2002": "gamma"}
		}`
		var reqSnake UpdateIssueRequest
		if err := json.Unmarshal([]byte(snake), &reqSnake); err != nil {
			t.Fatalf("snake_case Unmarshal failed: %v", err)
		}
		if !reflect.DeepEqual(reqSnake.AddLabels, []string{"urgent", "api"}) || !reflect.DeepEqual(reqSnake.RemoveLabels, []string{"wontfix"}) {
			t.Errorf("unexpected snake_case labels: %+v", reqSnake)
		}
		if !reflect.DeepEqual(reqSnake.FixVersions, []string{"3.0.0"}) || reqSnake.DueDate != "2026-12-15" {
			t.Errorf("unexpected snake_case fields: %+v", reqSnake)
		}
		if reqSnake.CustomFields["customfield_2002"] != "gamma" {
			t.Errorf("unexpected snake_case custom_fields: %v", reqSnake.CustomFields)
		}

		var bad UpdateIssueRequest
		if err := json.Unmarshal([]byte(`{"add_labels": "not-an-array"}`), &bad); err == nil {
			t.Fatal("expected error for invalid add_labels type")
		}
	})
}
