package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDumpSchemas(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jira-schemas-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

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
	logStartup("Test startup entry from unit test")
}
