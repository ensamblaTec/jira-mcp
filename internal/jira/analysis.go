package jira

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AnalyzeBacklog performs statistical and health analysis on a backlog.
func (c *Client) AnalyzeBacklog(ctx context.Context, boardID int, projectKey string) (*BacklogAnalysis, error) {
	var searchResult *SearchResult
	var err error

	if boardID > 0 {
		searchResult, err = c.GetBacklogIssues(ctx, boardID, 0, 100)
	} else if projectKey != "" {
		jql := fmt.Sprintf("project = '%s' AND statusCategory != Done AND (sprint is EMPTY OR sprint not in openSprints()) ORDER BY priority DESC, created ASC", projectKey)
		searchResult, err = c.SearchIssues(ctx, jql, nil, 0, 100)
	} else {
		return nil, fmt.Errorf("either boardId (> 0) or projectKey must be provided")
	}

	if err != nil {
		return nil, fmt.Errorf("fetching backlog issues for analysis: %w", err)
	}

	analysis := &BacklogAnalysis{
		TotalIssues:         len(searchResult.Issues),
		BreakdownByType:     make(map[string]int),
		BreakdownByPriority: make(map[string]int),
		TopLabels:           make(map[string]int),
		SampleStaleKeys:     make([]string, 0),
	}

	cutoff30Days := time.Now().AddDate(0, 0, -30)

	for _, issue := range searchResult.Issues {
		// Type breakdown
		if issue.Fields.IssueType != nil {
			analysis.BreakdownByType[issue.Fields.IssueType.Name]++
		}

		// Priority breakdown
		prio := "None"
		if issue.Fields.Priority != nil {
			prio = issue.Fields.Priority.Name
		}
		analysis.BreakdownByPriority[prio]++

		// Unassigned check
		if issue.Fields.Assignee == nil || issue.Fields.Assignee.DisplayName == "" {
			analysis.UnassignedIssues++
		}

		// Labels
		for _, lbl := range issue.Fields.Labels {
			analysis.TopLabels[lbl]++
		}

		// Stale check (updated > 30 days ago)
		if issue.Fields.Updated != "" {
			// Layout example: 2026-09-25T14:41:43.000+0000 or RFC3339
			t, errParse := time.Parse("2006-01-02T15:04:05.000-0700", issue.Fields.Updated)
			if errParse != nil {
				t, errParse = time.Parse(time.RFC3339, issue.Fields.Updated)
			}
			if errParse == nil && t.Before(cutoff30Days) {
				analysis.StaleIssuesCount++
				if len(analysis.SampleStaleKeys) < 10 {
					analysis.SampleStaleKeys = append(analysis.SampleStaleKeys, issue.Key)
				}
			}
		}
	}

	return analysis, nil
}

// AnalyzeSprint calculates completion rates, bottlenecks and workload distribution for a sprint.
func (c *Client) AnalyzeSprint(ctx context.Context, sprintID int) (*SprintAnalysis, error) {
	// First fetch sprint details
	sprintPath := c.AgilePath(fmt.Sprintf("sprint/%d", sprintID))
	var sprint Sprint
	if err := c.Get(ctx, sprintPath, nil, &sprint); err != nil {
		return nil, fmt.Errorf("getting sprint %d info: %w", sprintID, err)
	}

	searchResult, err := c.GetSprintIssues(ctx, sprintID, 0, 100)
	if err != nil {
		return nil, fmt.Errorf("getting issues for sprint %d: %w", sprintID, err)
	}

	analysis := &SprintAnalysis{
		SprintID:            sprint.ID,
		SprintName:          sprint.Name,
		State:               sprint.State,
		TotalIssues:         len(searchResult.Issues),
		WorkloadByAssignee:  make(map[string]int),
		BreakdownByType:     make(map[string]int),
		BreakdownByPriority: make(map[string]int),
	}

	for _, issue := range searchResult.Issues {
		// Type
		if issue.Fields.IssueType != nil {
			analysis.BreakdownByType[issue.Fields.IssueType.Name]++
		}

		// Priority
		prio := "None"
		if issue.Fields.Priority != nil {
			prio = issue.Fields.Priority.Name
		}
		analysis.BreakdownByPriority[prio]++

		// Status category
		if issue.Fields.Status != nil {
			cat := ""
			if issue.Fields.Status.StatusCategory != nil {
				cat = issue.Fields.Status.StatusCategory.Key
			}
			switch cat {
			case "done":
				analysis.CompletedIssues++
			case "indeterminate": // In Progress
				analysis.InProgressIssues++
			default:
				analysis.ToDoIssues++
			}
		}

		// Assignee workload
		assigneeName := "Unassigned"
		if issue.Fields.Assignee != nil && issue.Fields.Assignee.DisplayName != "" {
			assigneeName = issue.Fields.Assignee.DisplayName
		}
		analysis.WorkloadByAssignee[assigneeName]++
	}

	if analysis.TotalIssues > 0 {
		analysis.CompletionRate = (float64(analysis.CompletedIssues) / float64(analysis.TotalIssues)) * 100.0
	}

	return analysis, nil
}

// ExportIssues exports search results to a local file in json, csv, or markdown format.
func (c *Client) ExportIssues(ctx context.Context, jql string, format string, destPath string) (string, error) {
	result, err := c.SearchIssues(ctx, jql, nil, 0, 100)
	if err != nil {
		return "", fmt.Errorf("searching issues for export: %w", err)
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "json"
	}

	// Ensure parent directory exists
	parent := filepath.Dir(destPath)
	if parent != "" && parent != "." {
		if err := os.MkdirAll(parent, 0755); err != nil {
			return "", fmt.Errorf("creating directory for export file: %w", err)
		}
	}

	switch format {
	case "json":
		data, err := json.MarshalIndent(result.Issues, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshaling json: %w", err)
		}
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return "", fmt.Errorf("writing file: %w", err)
		}

	case "csv":
		file, err := os.Create(destPath)
		if err != nil {
			return "", fmt.Errorf("creating csv file: %w", err)
		}
		defer file.Close()

		writer := csv.NewWriter(file)
		defer writer.Flush()

		headers := []string{"Key", "Summary", "Type", "Status", "Priority", "Assignee", "Reporter", "Created", "Updated"}
		if err := writer.Write(headers); err != nil {
			return "", fmt.Errorf("writing csv headers: %w", err)
		}

		for _, issue := range result.Issues {
			issueType := ""
			if issue.Fields.IssueType != nil {
				issueType = issue.Fields.IssueType.Name
			}
			status := ""
			if issue.Fields.Status != nil {
				status = issue.Fields.Status.Name
			}
			prio := ""
			if issue.Fields.Priority != nil {
				prio = issue.Fields.Priority.Name
			}
			assignee := ""
			if issue.Fields.Assignee != nil {
				assignee = issue.Fields.Assignee.DisplayName
			}
			reporter := ""
			if issue.Fields.Reporter != nil {
				reporter = issue.Fields.Reporter.DisplayName
			}

			row := []string{
				issue.Key,
				issue.Fields.Summary,
				issueType,
				status,
				prio,
				assignee,
				reporter,
				issue.Fields.Created,
				issue.Fields.Updated,
			}
			if err := writer.Write(row); err != nil {
				return "", fmt.Errorf("writing csv row: %w", err)
			}
		}

	case "markdown", "md":
		var b strings.Builder
		b.WriteString(fmt.Sprintf("# Jira Export - JQL: `%s`\n\n", jql))
		b.WriteString(fmt.Sprintf("**Total Issues:** %d | **Date:** %s\n\n", len(result.Issues), time.Now().Format("2006-01-02 15:04:05")))
		b.WriteString("| Key | Summary | Type | Status | Priority | Assignee |\n")
		b.WriteString("|---|---|---|---|---|---|\n")

		for _, issue := range result.Issues {
			issueType := ""
			if issue.Fields.IssueType != nil {
				issueType = issue.Fields.IssueType.Name
			}
			status := ""
			if issue.Fields.Status != nil {
				status = issue.Fields.Status.Name
			}
			prio := ""
			if issue.Fields.Priority != nil {
				prio = issue.Fields.Priority.Name
			}
			assignee := "Unassigned"
			if issue.Fields.Assignee != nil && issue.Fields.Assignee.DisplayName != "" {
				assignee = issue.Fields.Assignee.DisplayName
			}
			summaryClean := strings.ReplaceAll(issue.Fields.Summary, "|", "\\|")

			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				issue.Key, summaryClean, issueType, status, prio, assignee))
		}

		if err := os.WriteFile(destPath, []byte(b.String()), 0644); err != nil {
			return "", fmt.Errorf("writing markdown file: %w", err)
		}

	default:
		return "", fmt.Errorf("unsupported export format '%s' (use json, csv, or markdown)", format)
	}

	return destPath, nil
}

// FormatAnalysisSummary creates a readable Markdown representation of BacklogAnalysis.
func (a *BacklogAnalysis) FormatAnalysisSummary() string {
	var b strings.Builder
	b.WriteString("### 📊 Backlog Analysis Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Total Items in Backlog:** %d\n", a.TotalIssues))
	b.WriteString(fmt.Sprintf("- **Unassigned Issues:** %d (%.1f%%)\n", a.UnassignedIssues, float64(a.UnassignedIssues)/float64(max(a.TotalIssues, 1))*100))
	b.WriteString(fmt.Sprintf("- **Stale Issues (> 30 days without updates):** %d\n\n", a.StaleIssuesCount))

	b.WriteString("#### 📌 Breakdown by Issue Type:\n")
	for k, v := range a.BreakdownByType {
		b.WriteString(fmt.Sprintf("- %s: %d\n", k, v))
	}

	b.WriteString("\n#### ⚡ Breakdown by Priority:\n")
	for k, v := range a.BreakdownByPriority {
		b.WriteString(fmt.Sprintf("- %s: %d\n", k, v))
	}

	if len(a.TopLabels) > 0 {
		b.WriteString("\n#### 🏷️ Common Labels:\n")
		type kv struct {
			k string
			v int
		}
		var sortedLabels []kv
		for k, v := range a.TopLabels {
			sortedLabels = append(sortedLabels, kv{k, v})
		}
		sort.Slice(sortedLabels, func(i, j int) bool { return sortedLabels[i].v > sortedLabels[j].v })
		for i, item := range sortedLabels {
			if i >= 5 {
				break
			}
			b.WriteString(fmt.Sprintf("- `%s`: %d\n", item.k, item.v))
		}
	}

	if len(a.SampleStaleKeys) > 0 {
		b.WriteString(fmt.Sprintf("\n⚠️ **Sample Stale Issues:** %s\n", strings.Join(a.SampleStaleKeys, ", ")))
	}

	return b.String()
}

// FormatSprintSummary creates a readable Markdown representation of SprintAnalysis.
func (s *SprintAnalysis) FormatSprintSummary() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("### 🏃 Sprint Analysis: %s (State: %s)\n\n", s.SprintName, s.State))
	b.WriteString(fmt.Sprintf("- **Total Issues:** %d\n", s.TotalIssues))
	b.WriteString(fmt.Sprintf("- **Completed (Done):** %d (%.1f%%)\n", s.CompletedIssues, s.CompletionRate))
	b.WriteString(fmt.Sprintf("- **In Progress:** %d\n", s.InProgressIssues))
	b.WriteString(fmt.Sprintf("- **To Do / Backlog:** %d\n\n", s.ToDoIssues))

	b.WriteString("#### 👥 Workload by Assignee:\n")
	for k, v := range s.WorkloadByAssignee {
		b.WriteString(fmt.Sprintf("- %s: %d issues\n", k, v))
	}

	b.WriteString("\n#### 📌 Breakdown by Issue Type:\n")
	for k, v := range s.BreakdownByType {
		b.WriteString(fmt.Sprintf("- %s: %d\n", k, v))
	}

	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
