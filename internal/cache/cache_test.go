package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ensamblatec/jira-mcp/internal/config"
	"github.com/ensamblatec/jira-mcp/internal/jira"
)

func TestFTS5Support(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jira-fts5-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	db, err := sql.Open("sqlite", filepath.Join(tmpDir, "fts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS test_fts USING fts5(key UNINDEXED, title, body);")
	if err != nil {
		t.Fatalf("FTS5 is not supported: %v", err)
	}

	_, err = db.Exec("INSERT INTO test_fts(key, title, body) VALUES ('KEY-1', 'title one', 'body content');")
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	_, err = db.Exec("INSERT INTO test_fts(key, title, body) VALUES ('KEY-2', 'Fix database deadlock', 'Occurs when login runs concurrently with sync');")
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	_, err = db.Exec("INSERT INTO test_fts(key, title, body) VALUES ('PROJ-123', 'Fix OAuth login', 'Details about PROJ-123 and deadlocks');")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	var key string
	// Test matching with hyphen
	err = db.QueryRow(`SELECT key FROM test_fts WHERE test_fts MATCH '"PROJ-123"'`).Scan(&key)
	if err != nil {
		t.Fatalf("Match hyphen failed: %v", err)
	}
	if key != "PROJ-123" {
		t.Fatalf("expected PROJ-123, got %s", key)
	}

	// Test snippet with column -1 (any matching column)
	var snippet string
	err = db.QueryRow(`SELECT snippet(test_fts, -1, '[', ']', '...', 10) FROM test_fts WHERE test_fts MATCH '"deadl"*'`).Scan(&snippet)
	if err != nil {
		t.Fatalf("Snippet column -1 failed: %v", err)
	}
	if !strings.Contains(snippet, "[deadlock") {
		t.Fatalf("expected snippet containing [deadlock, got '%s'", snippet)
	}
}

func TestCacheCRUDAndSearch(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jira-cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test-cache.db")
	c, err := New(dbPath)
	if err != nil {
		t.Fatalf("New cache error: %v", err)
	}
	defer c.Close()

	// 1. Upsert single issue
	issue1 := &jira.Issue{
		Key: "PROJ-1",
		ID:  "1001",
		Fields: jira.IssueFields{
			Summary:     "Fix database deadlock on login",
			Description: "Occurs under heavy load in postgres",
			Project:     &jira.Project{Key: "PROJ"},
			IssueType:   &jira.IssueType{Name: "Bug"},
			Status:      &jira.Status{Name: "In Progress", StatusCategory: &jira.StatusCategory{Name: "In Progress"}},
			Priority:    &jira.Priority{Name: "Highest"},
			Assignee:    &jira.User{AccountID: "acc-1", DisplayName: "Alice"},
			Labels:      []string{"backend", "db"},
			Created:     "2026-09-01T10:00:00.000+0000",
			Updated:     "2026-09-20T12:00:00.000+0000",
		},
	}

	if err := c.UpsertIssue(issue1); err != nil {
		t.Fatalf("UpsertIssue failed: %v", err)
	}

	// 2. Retrieve issue
	got, err := c.GetIssue("PROJ-1")
	if err != nil {
		t.Fatalf("GetIssue failed: %v", err)
	}
	if got == nil || got.Summary != "Fix database deadlock on login" {
		t.Errorf("expected summary 'Fix database deadlock on login', got %+v", got)
	}
	if len(got.Labels) != 2 || got.Labels[0] != "backend" {
		t.Errorf("expected labels [backend, db], got %v", got.Labels)
	}

	// 3. Batch Upsert
	issue2 := jira.Issue{
		Key: "PROJ-2",
		ID:  "1002",
		Fields: jira.IssueFields{
			Summary:   "Add biometric auth in mobile app",
			Project:   &jira.Project{Key: "PROJ"},
			IssueType: &jira.IssueType{Name: "Story"},
			Status:    &jira.Status{Name: "To Do"},
			Priority:  &jira.Priority{Name: "Medium"},
			Assignee:  &jira.User{DisplayName: "Bob"},
			Updated:   "2026-09-22T14:00:00.000+0000",
		},
	}
	issue3 := jira.Issue{
		Key: "BAR-1",
		ID:  "2001",
		Fields: jira.IssueFields{
			Summary:   "Setup CI runner",
			Project:   &jira.Project{Key: "BAR"},
			IssueType: &jira.IssueType{Name: "Task"},
			Status:    &jira.Status{Name: "Done"},
			Updated:   "2026-09-24T08:00:00.000+0000",
		},
	}

	if err := c.UpsertIssues([]jira.Issue{issue2, issue3}); err != nil {
		t.Fatalf("UpsertIssues batch failed: %v", err)
	}

	// 4. Search local cache
	// Search by keyword "deadlock"
	results, err := c.Search("deadlock", "", "", "", "", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 || results[0].Key != "PROJ-1" {
		t.Errorf("expected 1 result for 'deadlock' (PROJ-1), got %d", len(results))
	}

	// Search by project "PROJ"
	projResults, err := c.Search("", "PROJ", "", "", "", 10)
	if err != nil {
		t.Fatalf("Search by project failed: %v", err)
	}
	if len(projResults) != 2 {
		t.Errorf("expected 2 results for PROJ, got %d", len(projResults))
	}

	// Search by status "Done"
	doneResults, err := c.Search("", "", "Done", "", "", 10)
	if err != nil {
		t.Fatalf("Search by status failed: %v", err)
	}
	if len(doneResults) != 1 || doneResults[0].Key != "BAR-1" {
		t.Errorf("expected 1 Done issue (BAR-1), got %d", len(doneResults))
	}

	// 5. Stats
	stats, err := c.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalIssues != 3 {
		t.Errorf("expected 3 total issues, got %d", stats.TotalIssues)
	}
	if stats.ProjectBreakdown["PROJ"] != 2 || stats.ProjectBreakdown["BAR"] != 1 {
		t.Errorf("unexpected project breakdown: %+v", stats.ProjectBreakdown)
	}

	// 6. Delete
	if err := c.DeleteIssue("PROJ-1"); err != nil {
		t.Fatalf("DeleteIssue failed: %v", err)
	}
	deleted, err := c.GetIssue("PROJ-1")
	if err != nil {
		t.Fatalf("GetIssue after delete error: %v", err)
	}
	if deleted != nil {
		t.Errorf("expected nil after delete, got %+v", deleted)
	}
}

func TestCacheIncrementalSync(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/2/search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jira.SearchResult{
			Total: 1,
			Issues: []jira.Issue{
				{
					Key: "DEMO-1",
					Fields: jira.IssueFields{
						Summary: "Synced issue",
						Project: &jira.Project{Key: "DEMO"},
						Updated: "2026-09-25T15:00:00.000+0000",
					},
				},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jira.NewClient(&config.Config{
		BaseURL:    srv.URL,
		APIToken:   "tok",
		APIVersion: "2",
	})

	tmpDir, _ := os.MkdirTemp("", "jira-sync-test-*")
	defer os.RemoveAll(tmpDir)

	c, err := New(filepath.Join(tmpDir, "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	count, lastTs, err := c.SyncIncremental(context.Background(), client, "DEMO", false)
	if err != nil {
		t.Fatalf("SyncIncremental failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 issue synced, got %d", count)
	}
	if lastTs != "2026-09-25T15:00:00.000+0000" {
		t.Errorf("expected lastTs '2026-09-25T15:00:00.000+0000', got '%s'", lastTs)
	}

	saved, err := c.GetIssue("DEMO-1")
	if err != nil || saved == nil || saved.Summary != "Synced issue" {
		t.Errorf("expected cached issue DEMO-1, got %+v (err: %v)", saved, err)
	}
}

func TestContextSearchAndPack(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jira-context-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "context.db")
	c, err := New(dbPath)
	if err != nil {
		t.Fatalf("New cache failed: %v", err)
	}
	defer c.Close()

	epic := jira.Issue{
		Key: "PROJECT-1",
		Fields: jira.IssueFields{
			Summary:   "OAuth 2.0 Security Overhaul",
			Project:   &jira.Project{Key: "PROJ"},
			IssueType: &jira.IssueType{Name: "Epic"},
			Status:    &jira.Status{Name: "In Progress"},
			Priority:  &jira.Priority{Name: "High"},
		},
	}

	story1 := jira.Issue{
		Key: "PROJ-10",
		Fields: jira.IssueFields{
			Summary:     "Implement Refresh Token Rotation",
			Description: "Rotate refresh tokens on each token refresh request to prevent replay attacks.",
			Project:     &jira.Project{Key: "PROJ"},
			IssueType:   &jira.IssueType{Name: "Story"},
			Status:      &jira.Status{Name: "In Progress"},
			Priority:    &jira.Priority{Name: "Highest"},
			Assignee:    &jira.User{DisplayName: "Alice"},
			Parent:      &jira.Issue{Key: "PROJECT-1"},
			IssueLinks: []jira.IssueLink{
				{
					Type:        &jira.IssueLinkType{Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
					InwardIssue: &jira.Issue{Key: "ISSUE-5"},
				},
			},
		},
	}

	story2 := jira.Issue{
		Key: "PROJ-11",
		Fields: jira.IssueFields{
			Summary:   "Add PKCE support for mobile client",
			Project:   &jira.Project{Key: "PROJ"},
			IssueType: &jira.IssueType{Name: "Story"},
			Status:    &jira.Status{Name: "To Do"},
			Assignee:  &jira.User{DisplayName: "Bob"},
			Parent:    &jira.Issue{Key: "PROJECT-1"},
		},
	}

	subtask := jira.Issue{
		Key: "PROJ-12",
		Fields: jira.IssueFields{
			Summary:   "Unit tests for token rotation",
			Project:   &jira.Project{Key: "PROJ"},
			IssueType: &jira.IssueType{Name: "Sub-task"},
			Status:    &jira.Status{Name: "Done", StatusCategory: &jira.StatusCategory{Name: "Done"}},
			Parent:    &jira.Issue{Key: "PROJ-10"},
		},
	}

	if err := c.UpsertIssues([]jira.Issue{epic, story1, story2, subtask}); err != nil {
		t.Fatalf("UpsertIssues failed: %v", err)
	}

	// 1. Test FTS Context Search
	results, err := c.SearchContext("replay attacks", "", "", "", "", 10)
	if err != nil {
		t.Fatalf("SearchContext error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected matches for 'replay attacks', got 0")
	}
	if results[0].Key != "PROJ-10" {
		t.Errorf("expected PROJ-10 first result, got %s", results[0].Key)
	}
	if !strings.Contains(results[0].Snippet, "replay") {
		t.Errorf("expected snippet containing 'replay', got: %s", results[0].Snippet)
	}

	// 2. Test GetContextPack
	pack, err := c.GetContextPack(context.Background(), nil, "PROJ-10")
	if err != nil {
		t.Fatalf("GetContextPack failed: %v", err)
	}
	if pack == nil {
		t.Fatal("expected non-nil pack")
	}

	// Verify Parent
	if pack.Parent == nil || pack.Parent.Key != "PROJECT-1" {
		t.Errorf("expected Parent PROJECT-1, got %+v", pack.Parent)
	}

	// Verify Siblings (PROJ-11)
	if len(pack.Siblings) != 1 || pack.Siblings[0].Key != "PROJ-11" {
		t.Errorf("expected 1 sibling PROJ-11, got %d", len(pack.Siblings))
	}

	// Verify Subtasks (PROJ-12)
	if len(pack.Subtasks) != 1 || pack.Subtasks[0].Key != "PROJ-12" {
		t.Errorf("expected 1 subtask PROJ-12, got %d", len(pack.Subtasks))
	}

	// Verify BlockedBy (ISSUE-5)
	if len(pack.BlockedBy) != 1 || pack.BlockedBy[0] != "ISSUE-5" {
		t.Errorf("expected BlockedBy [ISSUE-5], got %v", pack.BlockedBy)
	}

	// 3. Test ToMarkdown
	md := pack.ToMarkdown()
	if !strings.Contains(md, "# [PROJ-10]") {
		t.Errorf("expected Markdown title '# [PROJ-10]', got: %s", md)
	}
	if !strings.Contains(md, "OAuth 2.0 Security Overhaul") {
		t.Errorf("expected Parent in markdown, got: %s", md)
	}
	if !strings.Contains(md, "PROJ-11") {
		t.Errorf("expected sibling PROJ-11 in markdown, got: %s", md)
	}
	if !strings.Contains(md, "[x] [PROJ-12]") {
		t.Errorf("expected completed subtask '[x] [PROJ-12]' in markdown, got: %s", md)
	}
	if !strings.Contains(md, "ISSUE-5") {
		t.Errorf("expected blocker ISSUE-5 in markdown, got: %s", md)
	}

	// 4. Verify Stats
	stats, err := c.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalIssues != 4 {
		t.Errorf("expected 4 total issues, got %d", stats.TotalIssues)
	}
	if stats.TotalRelations != 1 {
		t.Errorf("expected 1 relation, got %d", stats.TotalRelations)
	}
	if !stats.FTSIndexed {
		t.Errorf("expected FTSIndexed = true")
	}
}

func TestCleanFTSQuery_QuotesAndSpecialChars(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jira-fts-quotes-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	c, err := New(filepath.Join(tmpDir, "quotes.db"))
	if err != nil {
		t.Fatalf("New cache failed: %v", err)
	}
	defer c.Close()

	issue := jira.Issue{
		Key: "TEST-101",
		Fields: jira.IssueFields{
			Summary:     `Buy new monitor 24" for dev workstation`,
			Description: `Screen resolution 1920x1080 with HDMI and DisplayPort`,
			Project:     &jira.Project{Key: "TEST"},
			Status:      &jira.Status{Name: "Open"},
		},
	}
	if err := c.UpsertIssue(&issue); err != nil {
		t.Fatalf("UpsertIssue failed: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		minMatch int
	}{
		{
			name:     "internal quotes",
			input:    `monitor 24"`,
			minMatch: 1,
		},
		{
			name:     "double quotes wrapped",
			input:    `"workstation"`,
			minMatch: 1,
		},
		{
			name:     "syntax operators and parens",
			input:    `(HDMI) AND "DisplayPort"* : ^`,
			minMatch: 1,
		},
		{
			name:     "only punctuation",
			input:    `""" * : ^ ~ ()`,
			minMatch: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := CleanFTSQuery(tt.input)
			if strings.Contains(cleaned, `""`) {
				t.Errorf("cleaned query contains illegal double quotes: %q", cleaned)
			}
			results, err := c.SearchContext(tt.input, "", "", "", "", 10)
			if err != nil {
				t.Fatalf("SearchContext with %q caused SQL error: %v", tt.input, err)
			}
			if len(results) < tt.minMatch {
				t.Errorf("expected at least %d match, got %d", tt.minMatch, len(results))
			}
		})
	}
}

func BenchmarkSearchContextFTS(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "jira-bench-fts-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	c, err := New(filepath.Join(tmpDir, "bench.db"))
	if err != nil {
		b.Fatalf("New cache failed: %v", err)
	}
	defer c.Close()

	var issues []jira.Issue
	for i := 0; i < 100; i++ {
		issues = append(issues, jira.Issue{
			Key: fmt.Sprintf("BENCH-%d", i),
			Fields: jira.IssueFields{
				Summary:     fmt.Sprintf("Optimize database query performance iteration %d", i),
				Description: fmt.Sprintf("Investigating slow SQL query on table %d with heavy FTS indexing", i),
				Project:     &jira.Project{Key: "BENCH"},
				Status:      &jira.Status{Name: "Open"},
			},
		})
	}
	if err := c.UpsertIssues(issues); err != nil {
		b.Fatalf("UpsertIssues failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.SearchContext("database query performance", "BENCH", "", "", "", 10)
		if err != nil {
			b.Fatalf("SearchContext benchmark error: %v", err)
		}
	}
}

func BenchmarkGetContextPack(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "jira-bench-pack-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	c, err := New(filepath.Join(tmpDir, "bench_pack.db"))
	if err != nil {
		b.Fatalf("New cache failed: %v", err)
	}
	defer c.Close()

	epic := jira.Issue{
		Key: "PROJECT-1",
		Fields: jira.IssueFields{
			Summary: "Epic container",
			Project: &jira.Project{Key: "PROJECT"},
		},
	}
	story := jira.Issue{
		Key: "SAMPLE-1",
		Fields: jira.IssueFields{
			Summary: "Story with parent and subtasks",
			Project: &jira.Project{Key: "PROJECT"},
			Parent:  &jira.Issue{Key: "PROJECT-1"},
		},
	}
	sub := jira.Issue{
		Key: "TASK-1",
		Fields: jira.IssueFields{
			Summary: "Subtask 1",
			Project: &jira.Project{Key: "PROJECT"},
			Parent:  &jira.Issue{Key: "SAMPLE-1"},
		},
	}
	if err := c.UpsertIssues([]jira.Issue{epic, story, sub}); err != nil {
		b.Fatalf("UpsertIssues failed: %v", err)
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.GetContextPack(ctx, nil, "SAMPLE-1")
		if err != nil {
			b.Fatalf("GetContextPack benchmark error: %v", err)
		}
	}
}

func TestBoardPersistenceAndLocalView(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(filepath.Join(tmpDir, "board_test.db"))
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer c.Close()

	// 1. Test nil params guard
	if err := c.UpsertBoard(nil, nil); err == nil {
		t.Errorf("expected error for UpsertBoard(nil, nil), got nil")
	}

	// 2. Test UpsertBoard with full configuration
	b := &jira.Board{ID: 42, Name: "Core Platform Board", Type: "scrum"}
	cfg := &jira.BoardConfiguration{
		ID:   42,
		Name: "Core Platform Board",
		Type: "scrum",
		Location: &jira.BoardLocation{
			ProjectKey: "TEAM",
		},
		ColumnConfig: jira.ColumnConfig{
			Columns: []jira.BoardColumn{
				{Name: "To Do", Statuses: []jira.BoardColumnStatus{{ID: "1"}}},
				{Name: "In Progress", Statuses: []jira.BoardColumnStatus{{ID: "2"}}},
				{Name: "Done", Statuses: []jira.BoardColumnStatus{{ID: "3"}}},
			},
		},
	}
	if err := c.UpsertBoard(b, cfg); err != nil {
		t.Fatalf("UpsertBoard failed: %v", err)
	}

	// 3. Test GetBoard
	retrieved, err := c.GetBoard(42)
	if err != nil || retrieved == nil {
		t.Fatalf("GetBoard failed: %v, retrieved: %+v", err, retrieved)
	}
	if retrieved.Name != "Core Platform Board" || len(retrieved.ColumnConfig.Columns) != 3 {
		t.Errorf("unexpected retrieved board: %+v", retrieved)
	}

	// 4. Test ListCachedBoards
	boards, err := c.ListCachedBoards()
	if err != nil {
		t.Fatalf("ListCachedBoards failed: %v", err)
	}
	if len(boards) != 1 || boards[0].ID != 42 {
		t.Errorf("expected 1 board with ID 42, got %+v", boards)
	}

	// 5. Test Anti-Clobbering: upserting with empty columns should NOT wipe existing column config
	partialCfg := &jira.BoardConfiguration{
		ID:   42,
		Name: "Core Platform Board Renamed",
		Type: "scrum",
		Location: &jira.BoardLocation{
			ProjectKey: "TEAM",
		},
	}
	if err := c.UpsertBoard(nil, partialCfg); err != nil {
		t.Fatalf("partial UpsertBoard failed: %v", err)
	}
	afterPartial, err := c.GetBoard(42)
	if err != nil || afterPartial == nil {
		t.Fatalf("GetBoard after partial failed: %v", err)
	}
	if len(afterPartial.ColumnConfig.Columns) != 3 {
		t.Errorf("expected column config to be preserved (3 columns), got %d", len(afterPartial.ColumnConfig.Columns))
	}

	// 6. Test Seed issues
	issues := []jira.Issue{
		{Key: "TEAM-1", Fields: jira.IssueFields{Summary: "Task 1", Status: &jira.Status{Name: "To Do"}, Project: &jira.Project{Key: "TEAM"}}},
		{Key: "TEAM-2", Fields: jira.IssueFields{Summary: "Task 2", Status: &jira.Status{Name: "In Progress"}, Project: &jira.Project{Key: "TEAM"}}},
		{Key: "TEAM-3", Fields: jira.IssueFields{Summary: "Task 3", Status: &jira.Status{Name: "Done"}, Project: &jira.Project{Key: "TEAM"}}},
		{Key: "TEAM-4", Fields: jira.IssueFields{Summary: "Task 4", Status: &jira.Status{Name: "Backlog"}, Project: &jira.Project{Key: "TEAM"}}},
		{Key: "TEAM-5", Fields: jira.IssueFields{Summary: "Task 5", Status: &jira.Status{Name: "Review"}, Project: &jira.Project{Key: "TEAM"}}},
		{Key: "BAR-99", Fields: jira.IssueFields{Summary: "Secret ticket", Status: &jira.Status{Name: "To Do"}, Project: &jira.Project{Key: "BAR"}}},
	}
	if err := c.UpsertIssues(issues); err != nil {
		t.Fatalf("UpsertIssues failed: %v", err)
	}

	// 7. Test GetLocalBoardView with includeBacklog=true
	viewWithBacklog, err := c.GetLocalBoardView(42, "", nil, true)
	if err != nil {
		t.Fatalf("GetLocalBoardView failed: %v", err)
	}
	if viewWithBacklog.ProjectKey != "TEAM" {
		t.Errorf("expected ProjectKey 'TEAM', got '%s'", viewWithBacklog.ProjectKey)
	}
	// Verify other project ticket was NOT included
	for _, col := range viewWithBacklog.Columns {
		for _, card := range col.Cards {
			if strings.HasPrefix(card.Key, "BAR-") {
				t.Fatalf("cross-project leak! Found card %s in board view", card.Key)
			}
		}
	}
	for _, card := range viewWithBacklog.Backlog {
		if strings.HasPrefix(card.Key, "BAR-") {
			t.Fatalf("cross-project leak! Found card %s in backlog", card.Key)
		}
	}

	// Check backlog card
	if len(viewWithBacklog.Backlog) != 1 || viewWithBacklog.Backlog[0].Key != "TEAM-4" {
		t.Errorf("expected TEAM-4 in backlog, got %+v", viewWithBacklog.Backlog)
	}

	// Check Unmapped column
	foundUnmapped := false
	for _, col := range viewWithBacklog.Columns {
		if col.Name == "Unmapped" {
			foundUnmapped = true
			if len(col.Cards) != 1 || col.Cards[0].Key != "TEAM-5" {
				t.Errorf("expected TEAM-5 in Unmapped column, got %+v", col.Cards)
			}
		}
	}
	if !foundUnmapped {
		t.Errorf("expected Unmapped column to be created for 'Review' status")
	}

	// 8. Test GetLocalBoardView with includeBacklog=false
	viewNoBacklog, err := c.GetLocalBoardView(42, "", nil, false)
	if err != nil {
		t.Fatalf("GetLocalBoardView(false) failed: %v", err)
	}
	if len(viewNoBacklog.Backlog) != 0 {
		t.Errorf("expected 0 backlog cards when includeBacklog=false, got %d", len(viewNoBacklog.Backlog))
	}

	// 9. Test cross-project leak prevention when board not found and projectKey is empty
	_, err = c.GetLocalBoardView(9999, "", nil, true)
	if err == nil {
		t.Errorf("expected error when board is not cached and projectKey is empty, got nil")
	}
}
