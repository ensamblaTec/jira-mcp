package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ensamblatec/jira-mcp/internal/cache"
	"github.com/ensamblatec/jira-mcp/internal/config"
	"github.com/ensamblatec/jira-mcp/internal/jira"
)

func TestUIServer_Routes(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_ui.db")
	c, err := cache.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer c.Close()

	// Seed cached board and issue
	b := &jira.Board{ID: 100, Name: "Test Scrum Board", Type: "scrum"}
	cfg := &jira.BoardConfiguration{
		ID:   100,
		Name: "Test Scrum Board",
		Type: "scrum",
		Location: &jira.BoardLocation{
			ProjectKey: "TEST",
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
		t.Fatalf("failed to upsert board: %v", err)
	}

	appCfg := &config.Config{
		BaseURL: "https://example.atlassian.net",
	}
	srv := NewServer(nil, c, appCfg)
	handler := srv.Handler()

	t.Run("GET / returns index HTML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if contentType := rec.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
			t.Errorf("expected text/html, got %s", contentType)
		}
	})

	t.Run("GET /api/boards returns cached boards", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/boards", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var boards []jira.Board
		if err := json.Unmarshal(rec.Body.Bytes(), &boards); err != nil {
			t.Fatalf("unmarshaling boards: %v", err)
		}
		if len(boards) == 0 {
			t.Fatalf("expected at least 1 board, got 0")
		}
		if boards[0].ID != 100 {
			t.Errorf("expected board 100, got %d", boards[0].ID)
		}
	})

	t.Run("GET /api/board?id=100&local=true returns local BoardView", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/board?id=100&local=true", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var view jira.BoardView
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatalf("unmarshaling BoardView: %v", err)
		}
		if view.BoardID != 100 {
			t.Errorf("expected board ID 100, got %d", view.BoardID)
		}
		if len(view.Columns) != 3 {
			t.Errorf("expected 3 columns, got %d", len(view.Columns))
		}
		if view.Columns[0].Name != "To Do" {
			t.Errorf("expected column 'To Do', got '%s'", view.Columns[0].Name)
		}
	})

	t.Run("GET /browse/TEST-123 redirects to Jira Cloud", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/browse/TEST-123", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusTemporaryRedirect {
			t.Fatalf("expected 307 redirect, got %d", rec.Code)
		}
		loc := rec.Header().Get("Location")
		if loc != "https://example.atlassian.net/browse/TEST-123" {
			t.Errorf("expected redirect location, got %s", loc)
		}
	})

	t.Run("GET /browse/attacker.com rejects invalid issue key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/browse/attacker.com", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for invalid key redirect, got %d", rec.Code)
		}
	})

	t.Run("GET /api/boards on empty cache returns [] not null", func(t *testing.T) {
		emptyDir := t.TempDir()
		emptyCache, err := cache.New(filepath.Join(emptyDir, "empty.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer emptyCache.Close()
		emptySrv := NewServer(nil, emptyCache, appCfg)
		req := httptest.NewRequest(http.MethodGet, "/api/boards", nil)
		rec := httptest.NewRecorder()
		emptySrv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if body := rec.Body.String(); body != "[]\n" {
			t.Errorf("expected '[]\\n', got '%s'", body)
		}
	})
}
