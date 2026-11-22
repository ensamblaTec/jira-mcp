package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config holds all configuration options for Jira MCP Server.
type Config struct {
	BaseURL      string
	Email        string
	APIToken     string
	AuthType     string // "basic" or "bearer"
	APIVersion   string // "2" or "3", defaults to "2"
	Transport    string // "stdio" or "sse", defaults to "stdio"
	Port         int    // Port for SSE server, defaults to 8080
	Timeout      time.Duration
	Debug        bool
	CacheDBPath  string
	CacheEnabled bool
}

// Load loads configuration from environment variables, CLI flags (os.Args[1:]), and defaults.
func Load() (*Config, error) {
	return LoadFromArgs(os.Args[1:])
}

// LoadFromArgs loads configuration from environment variables and an explicit args slice.
func LoadFromArgs(args []string) (*Config, error) {
	defaultCachePath := getEnv("JIRA_CACHE_DB", "")
	if defaultCachePath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			defaultCachePath = home + "/.jira-mcp/cache.db"
		} else {
			defaultCachePath = "./.jira-cache.db"
		}
	}

	cfg := &Config{
		BaseURL:      getEnv("JIRA_BASE_URL", getEnv("JIRA_URL", "")),
		Email:        getEnv("JIRA_EMAIL", getEnv("JIRA_USERNAME", "")),
		APIToken:     getEnv("JIRA_API_TOKEN", getEnv("JIRA_TOKEN", getEnv("JIRA_PAT", ""))),
		AuthType:     getEnv("JIRA_AUTH_TYPE", ""),
		APIVersion:   getEnv("JIRA_API_VERSION", "2"),
		Transport:    getEnv("MCP_TRANSPORT", "stdio"),
		Port:         8080,
		Timeout:      30 * time.Second,
		Debug:        getEnv("JIRA_DEBUG", "false") == "true" || getEnv("DEBUG", "false") == "true",
		CacheDBPath:  defaultCachePath,
		CacheEnabled: getEnv("JIRA_CACHE_ENABLED", "true") != "false",
	}

	// CLI Flags (flags override environment variables)
	fs := flag.NewFlagSet("jira-mcp", flag.ContinueOnError)
	baseURLFlag := fs.String("url", "", "Jira Base URL (e.g. https://company.atlassian.net)")
	emailFlag := fs.String("email", "", "Jira account email or username")
	tokenFlag := fs.String("token", "", "Jira API token or Personal Access Token (PAT)")
	authTypeFlag := fs.String("auth-type", "", "Authentication type: 'basic' or 'bearer'")
	apiVersionFlag := fs.String("api-version", "", "Jira REST API version ('2' or '3', default '2')")
	transportFlag := fs.String("transport", "", "MCP transport type: 'stdio' or 'sse'")
	portFlag := fs.Int("port", 0, "HTTP port for SSE transport (default 8080)")
	debugFlag := fs.Bool("debug", false, "Enable verbose logging to stderr")
	cacheDBFlag := fs.String("cache-db", "", "Path to local SQLite cache database file")
	noCacheFlag := fs.Bool("no-cache", false, "Disable local SQLite cache")

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}

	if *cacheDBFlag != "" {
		cfg.CacheDBPath = *cacheDBFlag
	}
	if *noCacheFlag {
		cfg.CacheEnabled = false
	}

	if *baseURLFlag != "" {
		cfg.BaseURL = *baseURLFlag
	}
	if *emailFlag != "" {
		cfg.Email = *emailFlag
	}
	if *tokenFlag != "" {
		cfg.APIToken = *tokenFlag
	}
	if *authTypeFlag != "" {
		cfg.AuthType = *authTypeFlag
	}
	if *apiVersionFlag != "" {
		cfg.APIVersion = *apiVersionFlag
	}
	if *transportFlag != "" {
		cfg.Transport = *transportFlag
	}
	if *portFlag != 0 {
		cfg.Port = *portFlag
	}
	if *debugFlag {
		cfg.Debug = true
	}

	// Clean and validate URL
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	// Auto-detect AuthType if not explicitly specified
	if cfg.AuthType == "" {
		if cfg.Email != "" && cfg.APIToken != "" {
			cfg.AuthType = "basic"
		} else if cfg.APIToken != "" {
			cfg.AuthType = "bearer"
		}
	}

	return cfg, nil
}

// Validate checks that required configuration is present.
func (c *Config) Validate() error {
	if c.BaseURL == "" {
		return errors.New("missing Jira Base URL. Set JIRA_BASE_URL / JIRA_URL environment variable or pass --url flag")
	}

	parsedURL, err := url.ParseRequestURI(c.BaseURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return fmt.Errorf("invalid Jira Base URL '%s': must start with http:// or https://", c.BaseURL)
	}

	if c.APIToken == "" {
		return errors.New("missing Jira API Token / PAT. Set JIRA_API_TOKEN environment variable or pass --token flag")
	}

	if c.AuthType == "basic" && c.Email == "" {
		return errors.New("basic authentication requires both email and api-token. Set JIRA_EMAIL environment variable or pass --email flag")
	}

	if c.Transport != "stdio" && c.Transport != "sse" {
		return fmt.Errorf("invalid transport '%s': must be 'stdio' or 'sse'", c.Transport)
	}

	return nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
