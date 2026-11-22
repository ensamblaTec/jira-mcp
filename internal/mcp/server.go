package mcp

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ensamblatec/jira-mcp/internal/cache"
	"github.com/ensamblatec/jira-mcp/internal/config"
	"github.com/ensamblatec/jira-mcp/internal/jira"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the mark3labs MCP Server, the Jira client, and SQLite cache.
type Server struct {
	mcpServer *server.MCPServer
	jira      *jira.Client
	cfg       *config.Config
	cache     *cache.Cache
}

// NewServer initializes the MCP server and registers all Jira tools and resources.
func NewServer(jiraClient *jira.Client, cfg *config.Config) *Server {
	s := server.NewMCPServer(
		"jira-mcp",
		"1.1.0",
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(false),
		server.WithResourceCapabilities(false, false),
		server.WithInstructions("Comprehensive Jira MCP server providing issues, agile boards, sprints, backlog, bulk operations, transitions, worklogs, attachments, and local SQLite cache with incremental sync."),
	)

	var localCache *cache.Cache
	if cfg.CacheEnabled {
		c, err := cache.New(cfg.CacheDBPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to initialize local SQLite cache at '%s': %v\n", cfg.CacheDBPath, err)
		} else {
			localCache = c
		}
	}

	srv := &Server{
		mcpServer: s,
		jira:      jiraClient,
		cfg:       cfg,
		cache:     localCache,
	}

	srv.registerAllTools()
	srv.registerPrompts()
	return srv
}

// Close closes cache and resources.
func (s *Server) Close() error {
	if s.cache != nil {
		return s.cache.Close()
	}
	return nil
}

// MCPServer returns the underlying MCP server instance.
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcpServer
}

// Cache returns the SQLite cache instance if enabled.
func (s *Server) Cache() *cache.Cache {
	return s.cache
}

// Helper to return formatted JSON tool result.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("formatting response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// Helper to return error as a tool result message so the LLM can self-correct.
func errResult(err error) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(err.Error()), nil
}

// Helper to return markdown text result.
func textResult(msg string) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(msg), nil
}
