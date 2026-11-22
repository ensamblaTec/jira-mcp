package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ensamblatec/jira-mcp/internal/cache"
	"github.com/ensamblatec/jira-mcp/internal/config"
	"github.com/ensamblatec/jira-mcp/internal/jira"
	mcpinternal "github.com/ensamblatec/jira-mcp/internal/mcp"
	"github.com/ensamblatec/jira-mcp/internal/ui"
	"github.com/mark3labs/mcp-go/server"
)

var (
	version = "1.0.0"
	commit  = "none"
	date    = "unknown"
)

func sanitizeArgs(args []string) []string {
	sanitized := make([]string, len(args))
	for i, arg := range args {
		if strings.HasPrefix(arg, "--token=") {
			sanitized[i] = "--token=***"
		} else if strings.HasPrefix(arg, "-token=") {
			sanitized[i] = "-token=***"
		} else if i > 0 && (args[i-1] == "--token" || args[i-1] == "-t" || args[i-1] == "-token") {
			sanitized[i] = "***"
		} else {
			sanitized[i] = arg
		}
	}
	return sanitized
}

func logStartup(msg string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".jira-mcp")
	_ = os.MkdirAll(dir, 0755)
	logFile := filepath.Join(dir, "startup.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format(time.RFC3339)
	fmt.Fprintf(f, "[%s] %s\n", ts, msg)
}

func dumpSchemas(targetDir string) {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create target dir: %v\n", err)
		os.Exit(1)
	}

	dummyCfg := &config.Config{
		BaseURL:      "https://example.atlassian.net",
		APIToken:     "dummy",
		CacheEnabled: false,
	}
	dummyClient := jira.NewClient(dummyCfg)
	srv := mcpinternal.NewServer(dummyClient, dummyCfg)

	tools := srv.MCPServer().ListTools()
	count := 0
	for name, st := range tools {
		filePath := filepath.Join(targetDir, name+".json")
		data, err := json.Marshal(st.Tool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal tool %s: %v\n", name, err)
			continue
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", filePath, err)
			continue
		}
		count++
	}
	fmt.Fprintf(os.Stdout, "Exported %d tool schemas to %s\n", count, targetDir)
}

func main() {
	logStartup(fmt.Sprintf("Entering main(), os.Args: %v", sanitizeArgs(os.Args)))

	isUI := false
	uiPort := 8080

	// Check for early flags (version or dump-schemas)
	for i, arg := range os.Args[1:] {
		if arg == "-v" || arg == "--version" {
			fmt.Fprintf(os.Stdout, "jira-mcp version %s (commit: %s, date: %s)\n", version, commit, date)
			os.Exit(0)
		}
		if arg == "--dump-schemas" && i+2 < len(os.Args) {
			targetDir := os.Args[i+2]
			dumpSchemas(targetDir)
			os.Exit(0)
		}
		if arg == "ui" || arg == "--ui" || arg == "-ui" {
			isUI = true
		}
		if (arg == "--port" || arg == "-port" || arg == "-p") && i+2 < len(os.Args) {
			if p, pErr := strconv.Atoi(os.Args[i+2]); pErr == nil && p > 0 {
				uiPort = p
			}
		}
	}

	cfg, err := config.Load()
	if err != nil {
		logStartup(fmt.Sprintf("Error loading configuration: %v", err))
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	validationErr := cfg.Validate()
	if validationErr != nil && !isUI {
		logStartup(fmt.Sprintf("Configuration validation error: %v (args: %v, baseURL_set: %t, email_set: %t, token_set: %t)",
			validationErr, sanitizeArgs(os.Args), cfg.BaseURL != "", cfg.Email != "", cfg.APIToken != ""))
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", validationErr)
		fmt.Fprintf(os.Stderr, "\nRequired environment variables:\n")
		fmt.Fprintf(os.Stderr, "  JIRA_BASE_URL (e.g. https://company.atlassian.net)\n")
		fmt.Fprintf(os.Stderr, "  JIRA_EMAIL    (user email for Jira Cloud)\n")
		fmt.Fprintf(os.Stderr, "  JIRA_API_TOKEN (API token for Cloud or PAT for Data Center)\n")
		fmt.Fprintf(os.Stderr, "\nOptional environment variables:\n")
		fmt.Fprintf(os.Stderr, "  JIRA_AUTH_TYPE (basic | bearer, default: auto-detected)\n")
		fmt.Fprintf(os.Stderr, "  JIRA_API_VERSION (2 | 3, default: 2)\n")
		fmt.Fprintf(os.Stderr, "  MCP_TRANSPORT (stdio | sse, default: stdio)\n")
		os.Exit(1)
	}
	logStartup("Server started successfully")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown on interruption signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	if isUI {
		if cfg.Port > 0 && uiPort == 8080 {
			uiPort = cfg.Port
		}
		var localCache *cache.Cache
		if c, err := cache.New(cfg.CacheDBPath); err == nil {
			localCache = c
			defer localCache.Close()
		}

		var jiraClient *jira.Client
		modeDesc := "Local SQLite Cache (< 2ms)"
		if validationErr == nil {
			jiraClient = jira.NewClient(cfg)
			modeDesc = "Dual (Local SQLite + Jira Cloud API)"
		}

		uiServer := ui.NewServer(jiraClient, localCache, cfg)
		fmt.Fprintf(os.Stdout, "\n")
		fmt.Fprintf(os.Stdout, "╔════════════════════════════════════════════════════════════════╗\n")
		fmt.Fprintf(os.Stdout, "║                Jira Local Boards Web Viewer                    ║\n")
		fmt.Fprintf(os.Stdout, "║                                                                ║\n")
		fmt.Fprintf(os.Stdout, "║  URL: http://localhost:%-5d                                   ║\n", uiPort)
		fmt.Fprintf(os.Stdout, "║  Mode: %-55s ║\n", modeDesc)
		fmt.Fprintf(os.Stdout, "║  Features: Dynamic Scrum & Kanban Columns, Backlog, Live Sync  ║\n")
		fmt.Fprintf(os.Stdout, "║  Press Ctrl+C to stop                                          ║\n")
		fmt.Fprintf(os.Stdout, "╚════════════════════════════════════════════════════════════════╝\n\n")

		go func() {
			<-ctx.Done()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = uiServer.Shutdown(shutdownCtx)
		}()

		if err := uiServer.Start(uiPort); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "UI Server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	jiraClient := jira.NewClient(cfg)
	mcpServer := mcpinternal.NewServer(jiraClient, cfg)
	defer mcpServer.Close()

	if cfg.Transport == "sse" {
		addr := fmt.Sprintf(":%d", cfg.Port)
		if cfg.Debug {
			fmt.Fprintf(os.Stderr, "Starting Jira MCP SSE server on %s...\n", addr)
		}
		sseServer := server.NewSSEServer(mcpServer.MCPServer())
		go func() {
			<-ctx.Done()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = sseServer.Shutdown(shutdownCtx)
		}()
		if err := sseServer.Start(addr); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "SSE Server error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Default stdio transport for Claude Console, Claude Desktop, and Antigravity CLI (agy)
		if cfg.Debug {
			fmt.Fprintf(os.Stderr, "Starting Jira MCP stdio server...\n")
		}
		stdioServer := server.NewStdioServer(mcpServer.MCPServer())
		if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Stdio Server error: %v\n", err)
			os.Exit(1)
		}
	}
}
