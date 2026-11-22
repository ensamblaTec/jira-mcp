package ui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ensamblatec/jira-mcp/internal/cache"
	"github.com/ensamblatec/jira-mcp/internal/config"
	"github.com/ensamblatec/jira-mcp/internal/jira"
)

var jiraIssueKeyRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-[0-9]+$`)

//go:embed index.html
var indexHTML []byte

// Server provides the local web interface to visualize Scrum and Kanban agile boards.
type Server struct {
	jiraClient *jira.Client
	cache      *cache.Cache
	cfg        *config.Config
	server     *http.Server
}

// NewServer initializes the local UI server.
func NewServer(jiraClient *jira.Client, cache *cache.Cache, cfg *config.Config) *Server {
	return &Server{
		jiraClient: jiraClient,
		cache:      cache,
		cfg:        cfg,
	}
}

// Handler returns the HTTP handler for the UI routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/boards", s.handleListBoards)
	mux.HandleFunc("/api/board", s.handleGetBoard)
	mux.HandleFunc("/api/sync", s.handleSync)
	mux.HandleFunc("/browse/", s.handleBrowseRedirect)

	return mux
}

// Start launches the UI HTTP server on the loopback interface on the given port.
func (s *Server) Start(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 75 * time.Second,
	}

	return s.server.ListenAndServe()
}

// Shutdown stops the server gracefully.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/board" && r.URL.Path != "/boards" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (s *Server) handleListBoards(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	boards := make([]jira.Board, 0)
	var err error

	// Try live Jira API first
	if s.jiraClient != nil {
		boards, err = s.jiraClient.ListBoards(ctx, "", 0, 100)
	}

	// Fallback to cached boards if offline or API error
	if (err != nil || len(boards) == 0) && s.cache != nil {
		if cached, cErr := s.cache.ListCachedBoards(); cErr == nil && len(cached) > 0 {
			boards = cached
		}
	}

	if err != nil && len(boards) == 0 {
		http.Error(w, fmt.Sprintf("listing boards: %v", err), http.StatusInternalServerError)
		return
	}

	// Persist boards in cache for offline use
	if s.cache != nil && len(boards) > 0 {
		for _, b := range boards {
			_ = s.cache.UpsertBoard(&b, nil)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(boards)
}

func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	idStr := r.URL.Query().Get("id")
	boardID, err := strconv.Atoi(idStr)
	if err != nil || boardID <= 0 {
		http.Error(w, "invalid or missing 'id' query parameter", http.StatusBadRequest)
		return
	}

	localOnly := r.URL.Query().Get("local") == "true"
	var view *jira.BoardView

	if localOnly && s.cache != nil {
		view, err = s.cache.GetLocalBoardView(boardID, "", nil, true)
	} else if s.jiraClient != nil {
		view, err = s.jiraClient.BuildBoardView(ctx, boardID, true)
		if err != nil && s.cache != nil {
			// Fallback to local cache only if board exists in cache
			if cachedCfg, _ := s.cache.GetBoard(boardID); cachedCfg != nil {
				view, _ = s.cache.GetLocalBoardView(boardID, "", nil, true)
			}
		} else if s.cache != nil && view != nil {
			// Cache board configuration and issues
			boardCfg := &jira.BoardConfiguration{
				ID:   view.BoardID,
				Name: view.BoardName,
				Type: view.BoardType,
				Location: &jira.BoardLocation{
					ProjectKey: view.ProjectKey,
				},
			}
			_ = s.cache.UpsertBoard(nil, boardCfg)
		}
	} else if s.cache != nil {
		view, err = s.cache.GetLocalBoardView(boardID, "", nil, true)
	}

	if view == nil {
		http.Error(w, fmt.Sprintf("unable to load board %d: %v", boardID, err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(view)
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectKey := strings.TrimSpace(r.URL.Query().Get("project"))
	if projectKey == "" {
		http.Error(w, "missing 'project' query parameter", http.StatusBadRequest)
		return
	}

	if s.cache == nil || s.jiraClient == nil {
		http.Error(w, "cache or jira client not initialized", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	count, latest, err := s.cache.SyncIncremental(ctx, s.jiraClient, projectKey, false)
	if err != nil {
		http.Error(w, fmt.Sprintf("sync error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"project":           projectKey,
		"issuesSyncedCount": count,
		"latestTimestamp":   latest,
		"status":            "synchronized",
	})
}

func (s *Server) handleBrowseRedirect(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/browse/")
	if key == "" || !jiraIssueKeyRegex.MatchString(key) || s.cfg == nil || s.cfg.BaseURL == "" {
		http.NotFound(w, r)
		return
	}
	targetURL := fmt.Sprintf("%s/browse/%s", strings.TrimRight(s.cfg.BaseURL, "/"), key)
	http.Redirect(w, r, targetURL, http.StatusTemporaryRedirect)
}
