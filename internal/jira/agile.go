package jira

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ListBoards returns all agile boards, optionally filtered by type ("scrum" or "kanban").
func (c *Client) ListBoards(ctx context.Context, boardType string, startAt, maxResults int) ([]Board, error) {
	path := c.AgilePath("board")
	if maxResults <= 0 {
		maxResults = 50
	}

	q := url.Values{
		"startAt":    []string{strconv.Itoa(startAt)},
		"maxResults": []string{strconv.Itoa(maxResults)},
	}
	if boardType != "" {
		q.Set("type", boardType)
	}

	var resp BoardsResponse
	if err := c.Get(ctx, path, q, &resp); err != nil {
		return nil, fmt.Errorf("listing agile boards: %w", err)
	}

	return resp.Values, nil
}

// ListSprints returns sprints for a board, optionally filtered by state ("active", "future", "closed").
func (c *Client) ListSprints(ctx context.Context, boardID int, state string, startAt, maxResults int) ([]Sprint, error) {
	path := c.AgilePath(fmt.Sprintf("board/%d/sprint", boardID))
	if maxResults <= 0 {
		maxResults = 50
	}

	q := url.Values{
		"startAt":    []string{strconv.Itoa(startAt)},
		"maxResults": []string{strconv.Itoa(maxResults)},
	}
	if state != "" {
		q.Set("state", state)
	}

	var resp SprintsResponse
	if err := c.Get(ctx, path, q, &resp); err != nil {
		return nil, fmt.Errorf("listing sprints for board %d: %w", boardID, err)
	}

	return resp.Values, nil
}

// GetSprintIssues returns all issues in a specific sprint.
func (c *Client) GetSprintIssues(ctx context.Context, sprintID int, startAt, maxResults int) (*SearchResult, error) {
	path := c.AgilePath(fmt.Sprintf("sprint/%d/issue", sprintID))
	if maxResults <= 0 {
		maxResults = 50
	}

	q := url.Values{
		"startAt":    []string{strconv.Itoa(startAt)},
		"maxResults": []string{strconv.Itoa(maxResults)},
	}

	var resp SearchResult
	if err := c.Get(ctx, path, q, &resp); err != nil {
		return nil, fmt.Errorf("getting issues for sprint %d: %w", sprintID, err)
	}

	return &resp, nil
}

// GetBacklogIssues returns all issues currently sitting in the backlog of a board.
func (c *Client) GetBacklogIssues(ctx context.Context, boardID int, startAt, maxResults int) (*SearchResult, error) {
	path := c.AgilePath(fmt.Sprintf("board/%d/backlog", boardID))
	if maxResults <= 0 {
		maxResults = 50
	}

	q := url.Values{
		"startAt":    []string{strconv.Itoa(startAt)},
		"maxResults": []string{strconv.Itoa(maxResults)},
	}

	var resp SearchResult
	if err := c.Get(ctx, path, q, &resp); err != nil {
		return nil, fmt.Errorf("getting backlog issues for board %d: %w", boardID, err)
	}

	return &resp, nil
}

// CreateSprint creates a new sprint in a board.
func (c *Client) CreateSprint(ctx context.Context, boardID int, name, startDate, endDate, goal string) (*Sprint, error) {
	path := c.AgilePath("sprint")

	payload := map[string]any{
		"originBoardId": boardID,
		"name":          name,
	}
	if startDate != "" {
		payload["startDate"] = startDate
	}
	if endDate != "" {
		payload["endDate"] = endDate
	}
	if goal != "" {
		payload["goal"] = goal
	}

	var sprint Sprint
	if err := c.Post(ctx, path, payload, &sprint); err != nil {
		return nil, fmt.Errorf("creating sprint '%s' on board %d: %w", name, boardID, err)
	}

	return &sprint, nil
}

// UpdateSprintState updates the state of a sprint (e.g. "active" or "closed").
func (c *Client) UpdateSprintState(ctx context.Context, sprintID int, state string) error {
	path := c.AgilePath(fmt.Sprintf("sprint/%d", sprintID))

	payload := map[string]any{
		"state": state,
	}

	if err := c.Post(ctx, path, payload, nil); err != nil {
		return fmt.Errorf("updating sprint %d state to '%s': %w", sprintID, state, err)
	}

	return nil
}

// MoveIssuesToSprint moves a list of issue keys to a target sprint.
func (c *Client) MoveIssuesToSprint(ctx context.Context, sprintID int, issueKeys []string) error {
	if len(issueKeys) == 0 {
		return nil
	}
	path := c.AgilePath(fmt.Sprintf("sprint/%d/issue", sprintID))

	payload := map[string]any{
		"issues": issueKeys,
	}

	if err := c.Post(ctx, path, payload, nil); err != nil {
		return fmt.Errorf("moving issues to sprint %d: %w", sprintID, err)
	}

	return nil
}

// MoveIssuesToBacklog moves a list of issue keys back to the board backlog.
func (c *Client) MoveIssuesToBacklog(ctx context.Context, issueKeys []string) error {
	if len(issueKeys) == 0 {
		return nil
	}
	path := c.AgilePath("backlog/issue")

	payload := map[string]any{
		"issues": issueKeys,
	}

	if err := c.Post(ctx, path, payload, nil); err != nil {
		return fmt.Errorf("moving issues to backlog: %w", err)
	}

	return nil
}

// GetBoardConfiguration retrieves the column configuration, mapped statuses, and type of an agile board.
func (c *Client) GetBoardConfiguration(ctx context.Context, boardID int) (*BoardConfiguration, error) {
	path := c.AgilePath(fmt.Sprintf("board/%d/configuration", boardID))
	var config BoardConfiguration
	if err := c.Get(ctx, path, nil, &config); err != nil {
		return nil, fmt.Errorf("getting board %d configuration: %w", boardID, err)
	}
	return &config, nil
}

// GetBoardIssues returns issues associated with a board.
func (c *Client) GetBoardIssues(ctx context.Context, boardID int, startAt, maxResults int) (*SearchResult, error) {
	path := c.AgilePath(fmt.Sprintf("board/%d/issue", boardID))
	if maxResults <= 0 {
		maxResults = 50
	}
	q := url.Values{
		"startAt":    []string{strconv.Itoa(startAt)},
		"maxResults": []string{strconv.Itoa(maxResults)},
	}
	var resp SearchResult
	if err := c.Get(ctx, path, q, &resp); err != nil {
		return nil, fmt.Errorf("getting issues for board %d: %w", boardID, err)
	}
	return &resp, nil
}

// IssueToBoardCard converts a Jira Issue into a dense BoardCard.
func IssueToBoardCard(issue Issue) BoardCard {
	card := BoardCard{
		Key:       issue.Key,
		Summary:   issue.Fields.Summary,
		UpdatedAt: issue.Fields.Updated,
	}
	if issue.Fields.IssueType != nil {
		card.IssueType = issue.Fields.IssueType.Name
	}
	if issue.Fields.Status != nil {
		card.Status = issue.Fields.Status.Name
		if issue.Fields.Status.StatusCategory != nil {
			card.StatusCategory = issue.Fields.Status.StatusCategory.Name
		}
	}
	if issue.Fields.Priority != nil {
		card.Priority = issue.Fields.Priority.Name
	}
	if issue.Fields.Assignee != nil {
		card.AssigneeName = issue.Fields.Assignee.DisplayName
		card.AssigneeID = issue.Fields.Assignee.AccountID
	}
	if issue.Fields.Parent != nil {
		card.ParentKey = issue.Fields.Parent.Key
		if issue.Fields.Parent.Fields.IssueType != nil && strings.ToLower(issue.Fields.Parent.Fields.IssueType.Name) == "epic" {
			card.EpicKey = issue.Fields.Parent.Key
		}
	}
	if card.EpicKey == "" && issue.Fields.CustomFields != nil {
		for k, v := range issue.Fields.CustomFields {
			if strings.Contains(strings.ToLower(k), "epic") {
				if s, ok := v.(string); ok && strings.Contains(s, "-") {
					card.EpicKey = s
					break
				}
			}
		}
	}
	card.Labels = issue.Fields.Labels
	if len(issue.Fields.Components) > 0 {
		for _, comp := range issue.Fields.Components {
			card.Components = append(card.Components, comp.Name)
		}
	}
	return card
}

// BuildBoardView creates a fully populated BoardView with dynamic columns, cards, and backlog.
func (c *Client) BuildBoardView(ctx context.Context, boardID int, includeBacklog bool) (*BoardView, error) {
	cfg, err := c.GetBoardConfiguration(ctx, boardID)
	if err != nil {
		return nil, err
	}

	view := &BoardView{
		BoardID:       cfg.ID,
		BoardName:     cfg.Name,
		BoardType:     cfg.Type,
		ActiveSprints: []Sprint{},
		Columns:       make([]BoardColumnView, 0, len(cfg.ColumnConfig.Columns)),
		Backlog:       []BoardCard{},
	}
	if cfg.Location != nil {
		view.ProjectKey = cfg.Location.ProjectKey
	}

	// Setup columns and status mapping
	statusToCol := make(map[string]int)
	for i, col := range cfg.ColumnConfig.Columns {
		view.Columns = append(view.Columns, BoardColumnView{
			Name:  col.Name,
			Cards: []BoardCard{},
			Min:   col.Min,
			Max:   col.Max,
		})
		for _, st := range col.Statuses {
			statusToCol[st.ID] = i
		}
		// Also map by column name lowercase as fallback
		statusToCol[strings.ToLower(col.Name)] = i
	}

	// If no columns were configured, establish default workflow columns
	if len(view.Columns) == 0 {
		defaultCols := []string{"To Do", "In Progress", "Done"}
		for i, name := range defaultCols {
			view.Columns = append(view.Columns, BoardColumnView{
				Name:  name,
				Cards: []BoardCard{},
			})
			statusToCol[strings.ToLower(name)] = i
		}
	}

	// 1. Fetch Board Issues (Active Sprints for Scrum, or Board issues for Kanban)
	var mainIssues []Issue
	if strings.ToLower(cfg.Type) == "scrum" {
		sprints, err := c.ListSprints(ctx, boardID, "active", 0, 10)
		if err == nil && len(sprints) > 0 {
			view.ActiveSprint = &sprints[0]
			view.ActiveSprints = sprints
			seenKeys := make(map[string]struct{})
			for _, sp := range sprints {
				sprintIssues, err := c.GetSprintIssues(ctx, sp.ID, 0, 100)
				if err == nil && sprintIssues != nil {
					for _, iss := range sprintIssues.Issues {
						if _, seen := seenKeys[iss.Key]; !seen {
							seenKeys[iss.Key] = struct{}{}
							mainIssues = append(mainIssues, iss)
						}
					}
				}
			}
		}
		// In Scrum, if there are no active sprints, mainIssues remains empty.
		// Backlog issues will be fetched into view.Backlog below.
	} else {
		// Kanban board
		boardIssues, err := c.GetBoardIssues(ctx, boardID, 0, 200)
		if err == nil && boardIssues != nil {
			mainIssues = boardIssues.Issues
		}
	}

	// Populate board columns
	for _, iss := range mainIssues {
		card := IssueToBoardCard(iss)
		if view.ProjectKey == "" && iss.Fields.Project != nil {
			view.ProjectKey = iss.Fields.Project.Key
		}

		colIdx := -1
		if iss.Fields.Status != nil {
			// First try by Status ID
			if idx, ok := statusToCol[iss.Fields.Status.ID]; ok {
				colIdx = idx
			} else if idx, ok := statusToCol[strings.ToLower(iss.Fields.Status.Name)]; ok {
				colIdx = idx
			}
		}

		if colIdx >= 0 && colIdx < len(view.Columns) {
			view.Columns[colIdx].Cards = append(view.Columns[colIdx].Cards, card)
		} else {
			// Find or create "Unmapped" column rather than misplacing in column 0
			unmappedIdx := -1
			for i, col := range view.Columns {
				if col.Name == "Unmapped" {
					unmappedIdx = i
					break
				}
			}
			if unmappedIdx == -1 {
				view.Columns = append(view.Columns, BoardColumnView{
					Name:  "Unmapped",
					Cards: []BoardCard{card},
				})
			} else {
				view.Columns[unmappedIdx].Cards = append(view.Columns[unmappedIdx].Cards, card)
			}
		}
	}

	// 2. Fetch Backlog Issues if requested
	if includeBacklog {
		backlogResp, err := c.GetBacklogIssues(ctx, boardID, 0, 200)
		if err == nil && backlogResp != nil {
			for _, iss := range backlogResp.Issues {
				card := IssueToBoardCard(iss)
				view.Backlog = append(view.Backlog, card)
			}
		}
	}

	// Calculate total cards
	total := len(view.Backlog)
	for _, col := range view.Columns {
		total += len(col.Cards)
	}
	view.TotalCards = total

	return view, nil
}

func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return s
}

func sanitizeTableText(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// ToMarkdown renders the BoardView into a dense, human and LLM-readable Markdown document.
func (v *BoardView) ToMarkdown() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Board [%d]: %s\n\n", v.BoardID, v.BoardName))
	sb.WriteString(fmt.Sprintf("- **Type:** `%s` | **Project:** `%s` | **Total Items:** %d\n",
		strings.ToUpper(v.BoardType), v.ProjectKey, v.TotalCards))

	if len(v.ActiveSprints) > 1 {
		var sprintNames []string
		for _, sp := range v.ActiveSprints {
			sprintNames = append(sprintNames, fmt.Sprintf("%s (ID: %d)", sp.Name, sp.ID))
		}
		sb.WriteString(fmt.Sprintf("- **Active Sprints (%d):** %s\n", len(v.ActiveSprints), strings.Join(sprintNames, ", ")))
	} else if v.ActiveSprint != nil {
		sb.WriteString(fmt.Sprintf("- **Active Sprint:** %s (ID: %d | End: %s)\n",
			v.ActiveSprint.Name, v.ActiveSprint.ID, v.ActiveSprint.EndDate))
	}
	sb.WriteString("\n")

	// Render Columns
	sb.WriteString("## Board Columns (Active Workflow)\n\n")
	for _, col := range v.Columns {
		sb.WriteString(fmt.Sprintf("### %s (%d cards)\n\n", col.Name, len(col.Cards)))
		if len(col.Cards) == 0 {
			sb.WriteString("_No cards currently in this column_\n\n")
			continue
		}
		sb.WriteString("| Key | Type | Summary | Priority | Assignee |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		for _, card := range col.Cards {
			assignee := card.AssigneeName
			if assignee == "" {
				assignee = "Unassigned"
			}
			prio := card.Priority
			if prio == "" {
				prio = "None"
			}
			summary := sanitizeTableText(truncateRunes(card.Summary, 60))
			sb.WriteString(fmt.Sprintf("| **%s** | %s | %s | %s | %s |\n",
				card.Key, card.IssueType, summary, prio, assignee))
		}
		sb.WriteString("\n")
	}

	// Render Backlog
	if len(v.Backlog) > 0 {
		sb.WriteString(fmt.Sprintf("## Backlog (%d cards)\n\n", len(v.Backlog)))
		sb.WriteString("| Key | Type | Summary | Status | Priority | Assignee |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
		for _, card := range v.Backlog {
			assignee := card.AssigneeName
			if assignee == "" {
				assignee = "Unassigned"
			}
			summary := sanitizeTableText(truncateRunes(card.Summary, 60))
			sb.WriteString(fmt.Sprintf("| **%s** | %s | %s | %s | %s | %s |\n",
				card.Key, card.IssueType, summary, card.Status, card.Priority, assignee))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
