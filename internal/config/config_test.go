package config

import (
	"testing"
)

func clearJiraEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"JIRA_BASE_URL",
		"JIRA_URL",
		"JIRA_EMAIL",
		"JIRA_USERNAME",
		"JIRA_API_TOKEN",
		"JIRA_TOKEN",
		"JIRA_PAT",
		"JIRA_AUTH_TYPE",
		"JIRA_API_VERSION",
		"MCP_TRANSPORT",
		"JIRA_DEBUG",
		"DEBUG",
		"JIRA_CACHE_DB",
		"JIRA_CACHE_ENABLED",
	} {
		t.Setenv(key, "")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantError bool
	}{
		{
			name: "Valid Basic Auth",
			cfg: Config{
				BaseURL:    "https://example.atlassian.net",
				Email:      "user@example.com",
				APIToken:   "secret-token",
				AuthType:   "basic",
				APIVersion: "2",
				Transport:  "stdio",
			},
			wantError: false,
		},
		{
			name: "Valid Bearer Auth",
			cfg: Config{
				BaseURL:    "https://jira.example.com",
				APIToken:   "pat-token",
				AuthType:   "bearer",
				APIVersion: "2",
				Transport:  "stdio",
			},
			wantError: false,
		},
		{
			name: "Missing BaseURL",
			cfg: Config{
				Email:    "user@example.com",
				APIToken: "secret-token",
			},
			wantError: true,
		},
		{
			name: "Invalid BaseURL scheme",
			cfg: Config{
				BaseURL:  "ftp://example.atlassian.net",
				Email:    "user@example.com",
				APIToken: "secret-token",
			},
			wantError: true,
		},
		{
			name: "Missing Token",
			cfg: Config{
				BaseURL: "https://example.atlassian.net",
				Email:   "user@example.com",
			},
			wantError: true,
		},
		{
			name: "Basic Auth Missing Email",
			cfg: Config{
				BaseURL:  "https://example.atlassian.net",
				APIToken: "secret-token",
				AuthType: "basic",
			},
			wantError: true,
		},
		{
			name: "Invalid Transport",
			cfg: Config{
				BaseURL:   "https://example.atlassian.net",
				Email:     "user@example.com",
				APIToken:  "secret-token",
				AuthType:  "basic",
				Transport: "websocket",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestConfigLoadDefaults(t *testing.T) {
	clearJiraEnv(t)
	t.Setenv("JIRA_BASE_URL", "https://example.atlassian.net/")
	t.Setenv("JIRA_EMAIL", "test@example.com")
	t.Setenv("JIRA_API_TOKEN", "token123")

	cfg, err := LoadFromArgs([]string{})
	if err != nil {
		t.Fatalf("LoadFromArgs() unexpected error: %v", err)
	}

	if cfg.BaseURL != "https://example.atlassian.net" {
		t.Errorf("expected trimmed BaseURL 'https://example.atlassian.net', got '%s'", cfg.BaseURL)
	}
	if cfg.AuthType != "basic" {
		t.Errorf("expected AuthType 'basic', got '%s'", cfg.AuthType)
	}
	if cfg.APIVersion != "2" {
		t.Errorf("expected default APIVersion '2', got '%s'", cfg.APIVersion)
	}
	if cfg.Transport != "stdio" {
		t.Errorf("expected default Transport 'stdio', got '%s'", cfg.Transport)
	}
}

func TestConfigFlagOverrides(t *testing.T) {
	clearJiraEnv(t)
	t.Setenv("JIRA_BASE_URL", "https://your-domain.atlassian.net")
	t.Setenv("JIRA_EMAIL", "env@example.com")
	t.Setenv("JIRA_API_TOKEN", "env-token")

	args := []string{
		"--url", "https://placeholder.atlassian.net/",
		"--email", "flag@example.com",
		"--token", "flag-token",
		"--transport", "sse",
		"--port", "9090",
		"--debug",
	}

	cfg, err := LoadFromArgs(args)
	if err != nil {
		t.Fatalf("LoadFromArgs() unexpected error: %v", err)
	}

	if cfg.BaseURL != "https://placeholder.atlassian.net" {
		t.Errorf("expected flag BaseURL, got '%s'", cfg.BaseURL)
	}
	if cfg.Email != "flag@example.com" {
		t.Errorf("expected flag Email, got '%s'", cfg.Email)
	}
	if cfg.APIToken != "flag-token" {
		t.Errorf("expected flag Token, got '%s'", cfg.APIToken)
	}
	if cfg.Transport != "sse" {
		t.Errorf("expected Transport 'sse', got '%s'", cfg.Transport)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected Port 9090, got %d", cfg.Port)
	}
	if !cfg.Debug {
		t.Errorf("expected Debug to be true")
	}
}

func TestLoadFromArgsEdgeCases(t *testing.T) {
	t.Run("cache and auth/version flags override env", func(t *testing.T) {
		clearJiraEnv(t)
		t.Setenv("JIRA_URL", "https://example.atlassian.net")
		t.Setenv("JIRA_TOKEN", "fallback-token")

		cfg, err := LoadFromArgs([]string{
			"--no-cache",
			"--cache-db", "/tmp/custom-jira-cache.db",
			"--auth-type", "bearer",
			"--api-version", "3",
		})
		if err != nil {
			t.Fatalf("LoadFromArgs() unexpected error: %v", err)
		}
		if cfg.CacheEnabled {
			t.Errorf("expected CacheEnabled=false with --no-cache")
		}
		if cfg.CacheDBPath != "/tmp/custom-jira-cache.db" {
			t.Errorf("expected CacheDBPath '/tmp/custom-jira-cache.db', got %q", cfg.CacheDBPath)
		}
		if cfg.AuthType != "bearer" {
			t.Errorf("expected AuthType 'bearer', got %q", cfg.AuthType)
		}
		if cfg.APIVersion != "3" {
			t.Errorf("expected APIVersion '3', got %q", cfg.APIVersion)
		}
		if cfg.BaseURL != "https://example.atlassian.net" {
			t.Errorf("expected JIRA_URL fallback, got %q", cfg.BaseURL)
		}
		if cfg.APIToken != "fallback-token" {
			t.Errorf("expected JIRA_TOKEN fallback, got %q", cfg.APIToken)
		}
	})

	t.Run("JIRA_PAT fallback and bearer auto-detect", func(t *testing.T) {
		clearJiraEnv(t)
		t.Setenv("JIRA_BASE_URL", "https://jira.example.com")
		t.Setenv("JIRA_PAT", "pat-secret-value")

		cfg, err := LoadFromArgs(nil)
		if err != nil {
			t.Fatalf("LoadFromArgs() unexpected error: %v", err)
		}
		if cfg.APIToken != "pat-secret-value" {
			t.Errorf("expected JIRA_PAT fallback, got %q", cfg.APIToken)
		}
		if cfg.AuthType != "bearer" {
			t.Errorf("expected auto-detected AuthType 'bearer', got %q", cfg.AuthType)
		}
	})

	t.Run("JIRA_USERNAME fallback and basic auto-detect", func(t *testing.T) {
		clearJiraEnv(t)
		t.Setenv("JIRA_BASE_URL", "https://example.atlassian.net")
		t.Setenv("JIRA_USERNAME", "alice@example.com")
		t.Setenv("JIRA_API_TOKEN", "api-tok")

		cfg, err := LoadFromArgs(nil)
		if err != nil {
			t.Fatalf("LoadFromArgs() unexpected error: %v", err)
		}
		if cfg.Email != "alice@example.com" {
			t.Errorf("expected JIRA_USERNAME fallback, got %q", cfg.Email)
		}
		if cfg.AuthType != "basic" {
			t.Errorf("expected auto-detected AuthType 'basic', got %q", cfg.AuthType)
		}
	})

	t.Run("invalid flag returns error", func(t *testing.T) {
		clearJiraEnv(t)
		if _, err := LoadFromArgs([]string{"--non-existent-flag"}); err == nil {
			t.Fatalf("expected error on unknown flag, got nil")
		}
	})
}
