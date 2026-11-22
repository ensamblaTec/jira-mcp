package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ensamblatec/jira-mcp/internal/config"
	"github.com/ensamblatec/jira-mcp/internal/jira"
	"github.com/mark3labs/mcp-go/mcp"
)

func setupTestMCPServer(t *testing.T) (*httptest.Server, *Server, string) {
	mux := http.NewServeMux()

	// Myself
	mux.HandleFunc("/rest/api/2/myself", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.User{
			AccountID:    "acc-test",
			DisplayName:  "MCP Bot",
			EmailAddress: "bot@example.com",
		})
	})

	// Projects
	mux.HandleFunc("/rest/api/2/project", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]jira.Project{
			{ID: "1", Key: "TEST", Name: "Test Proj"},
		})
	})
	mux.HandleFunc("/rest/api/2/project/TEST", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.Project{
			ID: "1", Key: "TEST", Name: "Test Proj",
		})
	})
	mux.HandleFunc("/rest/api/2/project/TEST/versions", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]jira.Version{
			{ID: "v1", Name: "1.0.0"},
		})
	})
	mux.HandleFunc("/rest/api/2/version", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(jira.Version{
			ID: "v2", Name: "2.0.0"},
		)
	})

	// Issues
	mux.HandleFunc("/rest/api/2/issue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(jira.Issue{
				ID: "1001", Key: "TEST-1",
			})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/rest/api/2/issue/TEST-1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(jira.Issue{
				ID:  "1001",
				Key: "TEST-1",
				Fields: jira.IssueFields{
					Summary: "MCP Test Issue",
					Project: &jira.Project{Key: "TEST"},
					Status:  &jira.Status{Name: "To Do"},
					Created: "2026-09-01T10:00:00.000+0000",
					Updated: "2026-09-25T10:00:00.000+0000",
					Attachment: []jira.Attachment{
						{ID: "att-1", Filename: "sample.txt", Size: 15, Content: "/rest/api/2/attachment/content/att-1"},
					},
				},
			})
		case http.MethodPut, http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/rest/api/2/issue/TEST-1/comment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(jira.Comment{ID: "501", Body: "New Comment"})
			return
		}
		json.NewEncoder(w).Encode(jira.CommentsResponse{
			Total: 1,
			Comments: []jira.Comment{
				{ID: "501", Body: "Sample comment"},
			},
		})
	})

	mux.HandleFunc("/rest/api/2/issue/TEST-1/transitions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		json.NewEncoder(w).Encode(jira.TransitionsResponse{
			Transitions: []jira.Transition{
				{ID: "21", Name: "In Progress"},
			},
		})
	})

	mux.HandleFunc("/rest/api/2/issue/TEST-1/worklog", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(jira.Worklog{ID: "w-1", TimeSpent: "1h"})
			return
		}
		json.NewEncoder(w).Encode(jira.WorklogsResponse{
			Total: 1,
			Worklogs: []jira.Worklog{
				{ID: "w-1", TimeSpent: "1h"},
			},
		})
	})

	mux.HandleFunc("/rest/api/2/issue/TEST-1/changelog", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.ChangelogResponse{
			Total: 1,
			Histories: []jira.ChangelogHistory{
				{ID: "h-1"},
			},
		})
	})

	mux.HandleFunc("/rest/api/2/issue/TEST-1/attachments", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]jira.Attachment{
			{ID: "att-1", Filename: "sample.txt", Size: 15, Content: "/rest/api/2/attachment/content/att-1"},
		})
	})

	mux.HandleFunc("/rest/api/2/attachment/att-1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.Attachment{
			ID:       "att-1",
			Filename: "sample.txt",
			Size:     15,
			Content:  "/rest/api/2/attachment/content/att-1",
		})
	})

	mux.HandleFunc("/rest/api/2/attachment/content/att-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("mock attachment content"))
	})

	mux.HandleFunc("/rest/api/2/issue/TEST-1/assignee", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/rest/api/2/issueLink", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	// Search
	mux.HandleFunc("/rest/api/2/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.SearchResult{
			Total: 1,
			Issues: []jira.Issue{
				{
					Key: "TEST-1",
					ID:  "1001",
					Fields: jira.IssueFields{
						Summary: "MCP Test Issue",
						Project: &jira.Project{Key: "TEST"},
						Status:  &jira.Status{Name: "To Do"},
						Created: "2026-09-01T10:00:00.000+0000",
						Updated: "2026-09-25T10:00:00.000+0000",
					},
				},
			},
		})
	})

	// Meta / Fields / Types / Priorities / Users
	mux.HandleFunc("/rest/api/2/field", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]jira.Field{{ID: "summary", Name: "Summary"}})
	})
	mux.HandleFunc("/rest/api/2/issuetype", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]jira.IssueType{{ID: "1", Name: "Task"}})
	})
	mux.HandleFunc("/rest/api/2/priority", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]jira.Priority{{ID: "1", Name: "High"}})
	})
	mux.HandleFunc("/rest/api/2/user/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]jira.User{{AccountID: "acc-1", DisplayName: "Alice"}})
	})

	// Agile
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.BoardsResponse{
			Total:  1,
			Values: []jira.Board{{ID: 10, Name: "Test Board", Type: "scrum"}},
		})
	})
	mux.HandleFunc("/rest/agile/1.0/board/10/sprint", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.SprintsResponse{
			Total:  1,
			Values: []jira.Sprint{{ID: 100, Name: "Sprint 1", State: "active", OriginBoardID: 10}},
		})
	})
	mux.HandleFunc("/rest/agile/1.0/board/10/configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.BoardConfiguration{
			ID:   10,
			Name: "Test Board",
			Type: "scrum",
			Location: &jira.BoardLocation{
				ProjectKey: "TEST",
			},
			ColumnConfig: jira.ColumnConfig{
				Columns: []jira.BoardColumn{
					{Name: "To Do", Statuses: []jira.BoardColumnStatus{{ID: "1"}}},
					{Name: "Done", Statuses: []jira.BoardColumnStatus{{ID: "3"}}},
				},
			},
		})
	})
	mux.HandleFunc("/rest/agile/1.0/board/10/backlog", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.SearchResult{
			Total: 1,
			Issues: []jira.Issue{
				{
					Key: "TEST-1",
					Fields: jira.IssueFields{
						Summary: "Backlog Issue",
						Status:  &jira.Status{Name: "To Do"},
					},
				},
			},
		})
	})
	mux.HandleFunc("/rest/agile/1.0/sprint", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(jira.Sprint{ID: 101, Name: "Sprint 2", State: "future", OriginBoardID: 10})
	})
	mux.HandleFunc("/rest/agile/1.0/sprint/100", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(jira.Sprint{ID: 100, Name: "Sprint 1", State: "closed"})
			return
		}
		json.NewEncoder(w).Encode(jira.Sprint{ID: 100, Name: "Sprint 1", State: "active", OriginBoardID: 10})
	})
	mux.HandleFunc("/rest/agile/1.0/sprint/100/issue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		json.NewEncoder(w).Encode(jira.SearchResult{
			Total: 1,
			Issues: []jira.Issue{
				{
					Key: "TEST-1",
					Fields: jira.IssueFields{
						Summary: "Sprint Issue",
						Status:  &jira.Status{Name: "Done"},
					},
				},
			},
		})
	})
	mux.HandleFunc("/rest/agile/1.0/backlog/issue", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)

	tmpDir, _ := os.MkdirTemp("", "jira-mcp-test-*")
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := &config.Config{
		BaseURL:      srv.URL,
		Email:        "bot@example.com",
		APIToken:     "tok",
		AuthType:     "basic",
		APIVersion:   "2",
		Transport:    "stdio",
		CacheEnabled: true,
		CacheDBPath:  filepath.Join(tmpDir, "test-cache.db"),
	}

	jiraClient := jira.NewClient(cfg)
	mcpServer := NewServer(jiraClient, cfg)
	t.Cleanup(func() { mcpServer.Close() })

	return srv, mcpServer, tmpDir
}

func callTool(ctx context.Context, serverWrapper *Server, name string, args map[string]any) (*mcp.CallToolResult, error) {
	st := serverWrapper.MCPServer().GetTool(name)
	if st == nil {
		return nil, fmt.Errorf("tool %q not registered in MCPServer", name)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return st.Handler(ctx, req)
}

func TestMCPServer_AllToolsExecution(t *testing.T) {
	srv, serverWrapper, tmpDir := setupTestMCPServer(t)
	defer srv.Close()

	ctx := context.Background()

	// Upload file dummy
	dummyUploadFile := filepath.Join(tmpDir, "upload.txt")
	if err := os.WriteFile(dummyUploadFile, []byte("attachment test payload"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		tool    string
		args    map[string]any
		wantErr bool
	}{
		// Connection & Projects
		{name: "test_connection", tool: "jira_test_connection", args: nil},
		{name: "get_myself", tool: "jira_get_myself", args: nil},
		{name: "list_projects", tool: "jira_list_projects", args: nil},
		{name: "get_project", tool: "jira_get_project", args: map[string]any{"project_key": "TEST"}},

		// Issues
		{name: "create_issue", tool: "jira_create_issue", args: map[string]any{"project_key": "TEST", "issue_type": "Task", "summary": "MCP Issue", "description": "Line1\n\nLine2"}},
		{name: "get_issue", tool: "jira_get_issue", args: map[string]any{"issue_key": "TEST-1"}},
		{name: "search_issues", tool: "jira_search_issues", args: map[string]any{"jql": "project = TEST", "limit": 10}},
		{name: "update_issue", tool: "jira_update_issue", args: map[string]any{"issue_key": "TEST-1", "summary": "Updated Title"}},
		{name: "assign_issue", tool: "jira_assign_issue", args: map[string]any{"issue_key": "TEST-1", "assignee": "acc-1"}},
		{name: "create_subtask", tool: "jira_create_subtask", args: map[string]any{"parent_key": "TEST-1", "project_key": "TEST", "issue_type": "Sub-task", "summary": "Sub"}},
		{name: "get_issue_comments", tool: "jira_get_issue_comments", args: map[string]any{"issue_key": "TEST-1", "limit": 10}},
		{name: "add_comment", tool: "jira_add_comment", args: map[string]any{"issue_key": "TEST-1", "comment": "Comment text\n\nSecond paragraph"}},
		{name: "get_issue_transitions", tool: "jira_get_issue_transitions", args: map[string]any{"issue_key": "TEST-1"}},
		{name: "transition_issue", tool: "jira_transition_issue", args: map[string]any{"issue_key": "TEST-1", "transition": "21", "comment": "Moving to in progress"}},
		{name: "get_issue_worklogs", tool: "jira_get_issue_worklogs", args: map[string]any{"issue_key": "TEST-1"}},
		{name: "add_worklog", tool: "jira_add_worklog", args: map[string]any{"issue_key": "TEST-1", "time_spent": "2h", "comment": "Worked on tests"}},
		{name: "get_issue_links", tool: "jira_get_issue_links", args: map[string]any{"issue_key": "TEST-1"}},
		{name: "link_issues", tool: "jira_link_issues", args: map[string]any{"link_type": "Blocks", "inward_key": "TEST-1", "outward_key": "TEST-2"}},
		{name: "get_issue_changelog", tool: "jira_get_issue_changelog", args: map[string]any{"issue_key": "TEST-1", "limit": 10}},
		{name: "list_attachments", tool: "jira_list_attachments", args: map[string]any{"issue_key": "TEST-1"}},
		{name: "download_attachment", tool: "jira_download_attachment", args: map[string]any{"attachment_id": "att-1", "destination_path": filepath.Join(tmpDir, "downloaded.txt")}},
		{name: "upload_attachment", tool: "jira_upload_attachment", args: map[string]any{"issue_key": "TEST-1", "file_path": dummyUploadFile}},

		// Agile & Boards
		{name: "list_boards", tool: "jira_list_boards", args: nil},
		{name: "list_sprints", tool: "jira_list_sprints", args: map[string]any{"board_id": 10}},
		{name: "get_sprint_issues", tool: "jira_get_sprint_issues", args: map[string]any{"sprint_id": 100}},
		{name: "get_backlog_issues", tool: "jira_get_backlog_issues", args: map[string]any{"board_id": 10}},
		{name: "get_board_configuration", tool: "jira_get_board_configuration", args: map[string]any{"board_id": 10}},
		{name: "get_board_view", tool: "jira_get_board_view", args: map[string]any{"board_id": 10, "format": "markdown"}},
		{name: "create_sprint", tool: "jira_create_sprint", args: map[string]any{"board_id": 10, "name": "Sprint 2"}},
		{name: "update_sprint_state", tool: "jira_update_sprint_state", args: map[string]any{"sprint_id": 100, "state": "closed"}},
		{name: "move_issues_to_sprint", tool: "jira_move_issues_to_sprint", args: map[string]any{"sprint_id": 100, "issue_keys": []any{"TEST-1"}}},
		{name: "move_issues_to_backlog", tool: "jira_move_issues_to_backlog", args: map[string]any{"issue_keys": []any{"TEST-1"}}},
		{name: "analyze_backlog", tool: "jira_analyze_backlog", args: map[string]any{"board_id": 10}},
		{name: "analyze_sprint", tool: "jira_analyze_sprint", args: map[string]any{"sprint_id": 100}},

		// Bulk
		{name: "bulk_create_issues", tool: "jira_bulk_create_issues", args: map[string]any{"issues": []any{map[string]any{"project_key": "TEST", "issue_type": "Task", "summary": "Bulk 1"}}}},
		{name: "bulk_update_issues", tool: "jira_bulk_update_issues", args: map[string]any{"issue_keys": []any{"TEST-1"}, "priority": "High"}},
		{name: "bulk_transition_issues", tool: "jira_bulk_transition_issues", args: map[string]any{"issue_keys": []any{"TEST-1"}, "transition": "21"}},

		// Meta & Administration
		{name: "get_fields", tool: "jira_get_fields", args: nil},
		{name: "get_issue_types", tool: "jira_get_issue_types", args: nil},
		{name: "get_priorities", tool: "jira_get_priorities", args: nil},
		{name: "find_users", tool: "jira_find_users", args: map[string]any{"query": "Alice"}},
		{name: "get_epic_issues", tool: "jira_get_epic_issues", args: map[string]any{"epic_key": "TEST-1"}},
		{name: "list_versions", tool: "jira_list_versions", args: map[string]any{"project_key": "TEST"}},
		{name: "create_version", tool: "jira_create_version", args: map[string]any{"project_key": "TEST", "name": "2.0.0"}},
		{name: "export_issues", tool: "jira_export_issues", args: map[string]any{"jql": "project = TEST", "format": "json", "destination_path": filepath.Join(tmpDir, "exported.json")}},

		// Cache & Context Graph
		{name: "sync_cache", tool: "jira_sync_cache", args: map[string]any{"project_key": "TEST", "full_sync": true}},
		{name: "search_local_cache", tool: "jira_search_local_cache", args: map[string]any{"query": "MCP"}},
		{name: "get_cached_issue", tool: "jira_get_cached_issue", args: map[string]any{"issue_key": "TEST-1"}},
		{name: "cache_stats", tool: "jira_cache_stats", args: nil},
		{name: "context_search", tool: "jira_context_search", args: map[string]any{"query": `MCP "Test" Issue`}},
		{name: "get_context", tool: "jira_get_context", args: map[string]any{"issue_key": "TEST-1"}},

		// Deletion test at end
		{name: "delete_issue", tool: "jira_delete_issue", args: map[string]any{"issue_key": "TEST-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := callTool(ctx, serverWrapper, tt.tool, tt.args)
			if err != nil {
				t.Fatalf("unexpected call error on %s: %v", tt.tool, err)
			}
			if res == nil {
				t.Fatalf("expected non-nil result on %s", tt.tool)
			}
			if tt.wantErr && !res.IsError {
				t.Errorf("expected error result on %s, got success: %+v", tt.tool, res)
			}
			if !tt.wantErr && res.IsError {
				t.Errorf("tool %s returned unexpected error: %+v", tt.tool, res.Content)
			}
		})
	}
}

func TestMCPServer_MissingParameters(t *testing.T) {
	srv, serverWrapper, _ := setupTestMCPServer(t)
	defer srv.Close()

	ctx := context.Background()

	// Calling jira_get_issue without required issue_key should produce an error result
	res, err := callTool(ctx, serverWrapper, "jira_get_issue", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true when missing issue_key, got %+v", res)
	}

	// Calling jira_create_issue without summary
	res, err = callTool(ctx, serverWrapper, "jira_create_issue", map[string]any{
		"project_key": "TEST",
		"issue_type":  "Task",
	})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true when missing summary, got %+v", res)
	}
}

func TestServer_HelpersAndErrorHandling(t *testing.T) {
	// jsonResult with un-serializable channel
	res, err := jsonResult(make(chan int))
	if err != nil || !res.IsError {
		t.Errorf("expected error tool result for unmarshalable type, got %+v (err: %v)", res, err)
	}

	// errResult
	res, _ = errResult(fmt.Errorf("test error"))
	if !res.IsError {
		t.Errorf("expected isError=true")
	}

	// textResult
	res, _ = textResult("success")
	if res.IsError {
		t.Errorf("expected isError=false")
	}

	// Server with cache disabled
	cfg := &config.Config{
		BaseURL:      "http://localhost:12345",
		CacheEnabled: false,
	}
	srv := NewServer(jira.NewClient(cfg), cfg)
	if err := srv.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestMCPServer_ErgonomicFeatures(t *testing.T) {
	srv, serverWrapper, _ := setupTestMCPServer(t)
	defer srv.Close()

	ctx := context.Background()

	// 1. Create issue with template: "story" and assignee: "@me"
	res, err := callTool(ctx, serverWrapper, "jira_create_issue", map[string]any{
		"project_key": "TEST",
		"issue_type":  "Story",
		"summary":     "Story with template",
		"template":    "story",
		"assignee":    "@me",
		"labels":      []any{"ui", "frontend with spaces"},
	})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if res.IsError {
		t.Errorf("jira_create_issue with template failed: %+v", res.Content)
	}

	// 2. Create subtask with assignee: "@me"
	res, err = callTool(ctx, serverWrapper, "jira_create_subtask", map[string]any{
		"parent_key":  "TEST-1",
		"project_key": "TEST",
		"issue_type":  "Sub-task",
		"summary":     "Subtask with @me",
		"assignee":    "@me",
	})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if res.IsError {
		t.Errorf("jira_create_subtask with @me failed: %+v", res.Content)
	}

	// 3. Update issue with atomic add_labels and remove_labels, plus assignee: "@me"
	res, err = callTool(ctx, serverWrapper, "jira_update_issue", map[string]any{
		"issue_key":     "TEST-1",
		"add_labels":    []any{"backend", "v2-hotfix"},
		"remove_labels": []any{"wip", "draft"},
		"assignee":      "@me",
	})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if res.IsError {
		t.Errorf("jira_update_issue with atomic labels failed: %+v", res.Content)
	}

	// 4. Assign issue with assignee: "@me"
	res, err = callTool(ctx, serverWrapper, "jira_assign_issue", map[string]any{
		"issue_key": "TEST-1",
		"assignee":  "@me",
	})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if res.IsError {
		t.Errorf("jira_assign_issue with @me failed: %+v", res.Content)
	}

	// 5. Transition issue by name ("In Progress") and with custom fields
	res, err = callTool(ctx, serverWrapper, "jira_transition_issue", map[string]any{
		"issue_key":  "TEST-1",
		"transition": "In Progress",
		"comment":    "Starting work",
		"fields": map[string]any{
			"resolution": map[string]any{"name": "Done"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if res.IsError {
		t.Errorf("jira_transition_issue by name failed: %+v", res.Content)
	}
}

func TestMCPServer_Prompts(t *testing.T) {
	srv, serverWrapper, _ := setupTestMCPServer(t)
	defer srv.Close()

	ctx := context.Background()
	prompts := serverWrapper.MCPServer().ListPrompts()

	expectedPrompts := []string{
		"jira_create_story",
		"jira_report_bug",
		"jira_breakdown_feature",
	}

	for _, name := range expectedPrompts {
		sp, exists := prompts[name]
		if !exists {
			t.Errorf("expected prompt %q to be registered", name)
			continue
		}
		if sp.Handler == nil {
			t.Errorf("expected handler for prompt %q", name)
			continue
		}

		req := mcp.GetPromptRequest{}
		req.Params.Name = name
		req.Params.Arguments = map[string]string{
			"project_key":   "PROJ",
			"summary":       "User authentication feature",
			"parent_key":    "PROJ-10",
			"feature_title": "OAuth2 Login",
		}

		res, err := sp.Handler(ctx, req)
		if err != nil {
			t.Fatalf("prompt handler %q failed: %v", name, err)
		}
		if res == nil || len(res.Messages) == 0 {
			t.Fatalf("prompt %q returned empty messages", name)
		}
	}
}

func TestMCPServer_BoardToolsHardened(t *testing.T) {
	srv, serverWrapper, _ := setupTestMCPServer(t)
	defer srv.Close()

	ctx := context.Background()

	// 1. local_only: true on server with nil cache returns error
	noCacheWrapper := &Server{
		mcpServer: serverWrapper.mcpServer,
		jira:      serverWrapper.jira,
		cfg:       serverWrapper.cfg,
		cache:     nil,
	}
	res, err := callTool(ctx, noCacheWrapper, "jira_get_board_view", map[string]any{
		"board_id":   10,
		"local_only": true,
	})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected error when local_only=true and cache is nil")
	}

	// 2. Non-existent board (board_id: 9999) without cache should return error from Jira API, not a synthetic board
	res, err = callTool(ctx, serverWrapper, "jira_get_board_view", map[string]any{
		"board_id": 9999,
	})
	if err != nil {
		t.Fatalf("unexpected call error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected error for non-existent board 9999, got success: %+v", res)
	}
}
