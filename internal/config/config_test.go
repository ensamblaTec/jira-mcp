package config

import (
	"os"
	"testing"
)

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
	os.Clearenv()
	os.Setenv("JIRA_BASE_URL", "https://example.atlassian.net/")
	os.Setenv("JIRA_EMAIL", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "token123")

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
	os.Clearenv()
	os.Setenv("JIRA_BASE_URL", "https://your-domain.atlassian.net")
	os.Setenv("JIRA_EMAIL", "env@example.com")
	os.Setenv("JIRA_API_TOKEN", "env-token")

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
