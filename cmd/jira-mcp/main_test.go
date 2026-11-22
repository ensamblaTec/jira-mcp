package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDumpSchemas(t *testing.T) {
	tmpDir := t.TempDir()

	dumpSchemas(tmpDir)

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed reading dumped schemas dir: %v", err)
	}

	if len(entries) < 45 {
		t.Errorf("expected at least 45 schemas, got %d", len(entries))
	}

	// Verify at least one tool schema exists and is valid JSON
	samplePath := filepath.Join(tmpDir, "jira_get_issue.json")
	data, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("reading %s: %v", samplePath, err)
	}
	if len(data) == 0 {
		t.Errorf("expected non-empty schema file for %s", samplePath)
	}
}

func TestLogStartup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	const msg = "Test startup entry from unit test"
	logStartup(msg)

	logPath := filepath.Join(home, ".jira-mcp", "startup.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading isolated startup log %s: %v", logPath, err)
	}
	if !strings.Contains(string(data), msg) {
		t.Errorf("startup log %q does not contain %q", string(data), msg)
	}
}

func TestSanitizeArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "equals syntax long and short",
			args: []string{"jira-mcp", "--url", "https://example.atlassian.net", "--token=secret-1", "-token=secret-2", "--debug"},
			want: []string{"jira-mcp", "--url", "https://example.atlassian.net", "--token=***", "-token=***", "--debug"},
		},
		{
			name: "separate value flags",
			args: []string{"jira-mcp", "--token", "secret-a", "-t", "secret-b", "-token", "secret-c", "--port", "9090"},
			want: []string{"jira-mcp", "--token", "***", "-t", "***", "-token", "***", "--port", "9090"},
		},
		{
			name: "trailing token flag without value",
			args: []string{"jira-mcp", "--token"},
			want: []string{"jira-mcp", "--token"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeArgs(tc.args)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("sanitizeArgs(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
