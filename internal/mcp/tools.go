package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ensamblatec/jira-mcp/internal/jira"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerAllTools() {
	s.registerConnectionTools()
	s.registerIssueReadTools()
	s.registerIssueWriteTools()
	s.registerAgileTools()
	s.registerAnalysisTools()
	s.registerBulkTools()
	s.registerAttachmentTools()
	s.registerMetadataTools()
	s.registerCacheTools()
}

// -----------------------------------------------------------------------------
// Connection & Project Tools
// -----------------------------------------------------------------------------

func (s *Server) registerConnectionTools() {
	s.mcpServer.AddTool(mcp.NewTool("jira_test_connection",
		mcp.WithDescription("Verifies the Jira connection and credentials, returning current user information."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user, err := s.jira.GetMyself(ctx)
		if err != nil {
			return errResult(fmt.Errorf("connection failed: %w", err))
		}
		return jsonResult(map[string]any{
			"status":      "connected",
			"baseUrl":     s.cfg.BaseURL,
			"apiVersion":  s.cfg.APIVersion,
			"authType":    s.cfg.AuthType,
			"currentUser": user,
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_myself",
		mcp.WithDescription("Gets the profile and account details of the authenticated Jira user."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user, err := s.jira.GetMyself(ctx)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(user)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_list_projects",
		mcp.WithDescription("Lists all accessible Jira projects."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projects, err := s.jira.ListProjects(ctx)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(projects)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_project",
		mcp.WithDescription("Gets details, issue types, and components of a specific project."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("The key or ID of the project (e.g. 'PROJ')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("project_key")
		if err != nil {
			return errResult(err)
		}
		proj, err := s.jira.GetProject(ctx, key)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(proj)
	})
}

// -----------------------------------------------------------------------------
// Issue Read Tools
// -----------------------------------------------------------------------------

func (s *Server) registerIssueReadTools() {
	s.mcpServer.AddTool(mcp.NewTool("jira_get_issue",
		mcp.WithDescription("Gets full details of an issue by key (e.g. PROJ-123) including fields, status, assignee, and comments."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithArray("fields", mcp.Description("Optional list of specific field names to return (defaults to all)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		fields := req.GetStringSlice("fields", nil)
		issue, err := s.jira.GetIssue(ctx, key, fields)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(issue)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_search_issues",
		mcp.WithDescription("Search for issues using Jira Query Language (JQL) with pagination."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("jql", mcp.Required(), mcp.Description("JQL query string (e.g. 'project = PROJ AND status = \"In Progress\" ORDER BY priority DESC')")),
		mcp.WithInteger("limit", mcp.Description("Maximum number of issues to return (default 50, max 100)")),
		mcp.WithInteger("start_at", mcp.Description("Index of the first issue to return for pagination (default 0)")),
		mcp.WithArray("fields", mcp.Description("Optional list of field names to retrieve")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		jql, err := req.RequireString("jql")
		if err != nil {
			return errResult(err)
		}
		limit := req.GetInt("limit", 50)
		startAt := req.GetInt("start_at", 0)
		fields := req.GetStringSlice("fields", nil)

		result, err := s.jira.SearchIssues(ctx, jql, fields, startAt, limit)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(result)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_issue_comments",
		mcp.WithDescription("Lists all comments posted on a specific issue with pagination protection."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithInteger("limit", mcp.Description("Maximum comments to return (default 50, max 100)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		limit := req.GetInt("limit", 50)
		comments, err := s.jira.GetComments(ctx, key, limit)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(comments)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_issue_transitions",
		mcp.WithDescription("Lists all workflow transitions currently available for an issue (e.g. 'Start Progress', 'Done')."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		transitions, err := s.jira.GetTransitions(ctx, key)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(transitions)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_issue_worklogs",
		mcp.WithDescription("Retrieves worklog and time-spent history logged on an issue."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		worklogs, err := s.jira.GetWorklogs(ctx, key)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(worklogs)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_issue_links",
		mcp.WithDescription("Retrieves all linked/related issues (e.g. blocks, is blocked by, relates to)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		links, err := s.jira.GetIssueLinks(ctx, key)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(links)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_issue_changelog",
		mcp.WithDescription("Retrieves the audit trail and history of changes made to an issue with pagination protection."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithInteger("limit", mcp.Description("Maximum changelog entries to return (default 25, max 50)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		limit := req.GetInt("limit", 25)
		changelog, err := s.jira.GetChangelog(ctx, key, limit)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(changelog)
	})
}

// -----------------------------------------------------------------------------
// Issue Write & Update Tools
// -----------------------------------------------------------------------------

func (s *Server) registerIssueWriteTools() {
	s.mcpServer.AddTool(mcp.NewTool("jira_create_issue",
		mcp.WithDescription("Creates a new Jira card (Story, Task, Bug, Epic) in a project."),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("Project key (e.g. 'PROJ')")),
		mcp.WithString("issue_type", mcp.Required(), mcp.Description("Type of issue (e.g. 'Story', 'Task', 'Bug', 'Epic')")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Issue title / summary")),
		mcp.WithString("description", mcp.Description("Detailed description for the issue (Markdown supported)")),
		mcp.WithString("template", mcp.Description("Optional engineering template to prefill description if empty: 'story', 'bug', or 'spike'")),
		mcp.WithString("priority", mcp.Description("Priority name (e.g. 'Highest', 'High', 'Medium', 'Low')")),
		mcp.WithString("assignee", mcp.Description("Assignee accountId, username, or '@me' to assign to yourself")),
		mcp.WithArray("labels", mcp.Description("List of label strings")),
		mcp.WithArray("components", mcp.Description("List of component names")),
		mcp.WithString("due_date", mcp.Description("Due date in YYYY-MM-DD format")),
		mcp.WithString("parent_key", mcp.Description("Parent issue key if creating a subtask")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pKey, err := req.RequireString("project_key")
		if err != nil {
			return errResult(err)
		}
		iType, err := req.RequireString("issue_type")
		if err != nil {
			return errResult(err)
		}
		summary, err := req.RequireString("summary")
		if err != nil {
			return errResult(err)
		}

		createReq := jira.CreateIssueRequest{
			ProjectKey:  pKey,
			IssueType:   iType,
			Summary:     summary,
			Description: req.GetString("description", ""),
			Template:    req.GetString("template", ""),
			Priority:    req.GetString("priority", ""),
			Assignee:    req.GetString("assignee", ""),
			Labels:      req.GetStringSlice("labels", nil),
			Components:  req.GetStringSlice("components", nil),
			DueDate:     req.GetString("due_date", ""),
			ParentKey:   req.GetString("parent_key", ""),
		}

		created, err := s.jira.CreateIssue(ctx, createReq)
		if err != nil {
			return errResult(err)
		}

		if s.cache != nil {
			if fullIssue, errGet := s.jira.GetIssue(ctx, created.Key, nil); errGet == nil {
				_ = s.cache.UpsertIssue(fullIssue)
			}
		}

		return jsonResult(map[string]any{
			"status": "created",
			"key":    created.Key,
			"id":     created.ID,
			"url":    fmt.Sprintf("%s/browse/%s", s.cfg.BaseURL, created.Key),
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_create_subtask",
		mcp.WithDescription("Creates a subtask directly attached to a parent issue."),
		mcp.WithString("parent_key", mcp.Required(), mcp.Description("Parent issue key (e.g. 'PROJ-123')")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Subtask title / summary")),
		mcp.WithString("description", mcp.Description("Detailed subtask description")),
		mcp.WithString("assignee", mcp.Description("Assignee accountId, username, or '@me' to assign to yourself")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		parentKey, err := req.RequireString("parent_key")
		if err != nil {
			return errResult(err)
		}
		summary, err := req.RequireString("summary")
		if err != nil {
			return errResult(err)
		}

		// Retrieve parent issue to get its project key
		parentIssue, err := s.jira.GetIssue(ctx, parentKey, []string{"project"})
		if err != nil {
			return errResult(fmt.Errorf("retrieving parent issue: %w", err))
		}

		projectKey := ""
		if parentIssue.Fields.Project != nil {
			projectKey = parentIssue.Fields.Project.Key
		}
		if projectKey == "" && strings.Contains(parentKey, "-") {
			projectKey = strings.Split(parentKey, "-")[0]
		}

		createReq := jira.CreateIssueRequest{
			ProjectKey:  projectKey,
			IssueType:   "Sub-task",
			Summary:     summary,
			Description: req.GetString("description", ""),
			Assignee:    req.GetString("assignee", ""),
			ParentKey:   parentKey,
		}

		created, err := s.jira.CreateIssue(ctx, createReq)
		if err != nil {
			return errResult(err)
		}

		if s.cache != nil {
			if fullIssue, errGet := s.jira.GetIssue(ctx, created.Key, nil); errGet == nil {
				_ = s.cache.UpsertIssue(fullIssue)
			}
		}

		return jsonResult(map[string]any{
			"status":    "created",
			"key":       created.Key,
			"parentKey": parentKey,
			"url":       fmt.Sprintf("%s/browse/%s", s.cfg.BaseURL, created.Key),
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_update_issue",
		mcp.WithDescription("Updates fields of an existing Jira issue."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithString("summary", mcp.Description("New title / summary")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("priority", mcp.Description("New priority name")),
		mcp.WithString("assignee", mcp.Description("New assignee accountId, username, or '@me' ('none' or 'unassigned' to clear)")),
		mcp.WithArray("labels", mcp.Description("Full replacement list of labels (takes precedence over add_labels/remove_labels)")),
		mcp.WithArray("add_labels", mcp.Description("List of labels to append atomically without overwriting existing labels")),
		mcp.WithArray("remove_labels", mcp.Description("List of labels to remove atomically")),
		mcp.WithArray("components", mcp.Description("Replacement list of components")),
		mcp.WithString("due_date", mcp.Description("Due date (YYYY-MM-DD)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}

		updateReq := jira.UpdateIssueRequest{
			Summary:      req.GetString("summary", ""),
			Description:  req.GetString("description", ""),
			Priority:     req.GetString("priority", ""),
			Assignee:     req.GetString("assignee", ""),
			Labels:       req.GetStringSlice("labels", nil),
			AddLabels:    req.GetStringSlice("add_labels", nil),
			RemoveLabels: req.GetStringSlice("remove_labels", nil),
			Components:   req.GetStringSlice("components", nil),
			DueDate:      req.GetString("due_date", ""),
		}

		if err := s.jira.UpdateIssue(ctx, key, updateReq); err != nil {
			return errResult(err)
		}

		if s.cache != nil {
			if fullIssue, errGet := s.jira.GetIssue(ctx, key, nil); errGet == nil {
				_ = s.cache.UpsertIssue(fullIssue)
			}
		}

		return textResult(fmt.Sprintf("Issue %s updated successfully.", key))
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_transition_issue",
		mcp.WithDescription("Transitions an issue workflow state (e.g. to 'In Progress', 'In Review', 'Done'). Supports transition IDs or exact/case-insensitive status/transition names."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithString("transition", mcp.Required(), mcp.Description("Transition ID (e.g. '21') or exact state/transition name (e.g. 'DEPLOY', 'Revisar', 'Done'). The server automatically resolves names.")),
		mcp.WithString("comment", mcp.Description("Optional comment explaining the transition")),
		mcp.WithObject("fields", mcp.Description("Optional field updates required by the transition screen, such as resolution or fixVersions")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		target, err := req.RequireString("transition")
		if err != nil {
			return errResult(err)
		}
		comment := req.GetString("comment", "")

		var fieldsMap map[string]any
		if args := req.GetArguments(); args != nil {
			if f, ok := args["fields"].(map[string]any); ok {
				fieldsMap = f
			}
		}

		if err := s.jira.TransitionIssue(ctx, key, target, comment, fieldsMap); err != nil {
			return errResult(err)
		}

		if s.cache != nil {
			if fullIssue, errGet := s.jira.GetIssue(ctx, key, nil); errGet == nil {
				_ = s.cache.UpsertIssue(fullIssue)
			}
		}

		return textResult(fmt.Sprintf("Issue %s transitioned to '%s' successfully.", key, target))
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_assign_issue",
		mcp.WithDescription("Assigns an issue to a user by accountId, username, or '@me' (pass empty, 'unassigned', or 'none' to clear)."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithString("assignee", mcp.Required(), mcp.Description("User accountId, username, or '@me' to assign to yourself ('unassigned' or 'none' to clear)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		assignee, err := req.RequireString("assignee")
		if err != nil {
			return errResult(err)
		}

		if err := s.jira.AssignIssue(ctx, key, assignee); err != nil {
			return errResult(err)
		}

		if s.cache != nil {
			if fullIssue, errGet := s.jira.GetIssue(ctx, key, nil); errGet == nil {
				_ = s.cache.UpsertIssue(fullIssue)
			}
		}

		return textResult(fmt.Sprintf("Issue %s assigned to '%s'.", key, assignee))
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_add_comment",
		mcp.WithDescription("Adds a new comment to an issue."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithString("comment", mcp.Required(), mcp.Description("Comment body (Markdown supported)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		commentBody, err := req.RequireString("comment")
		if err != nil {
			return errResult(err)
		}

		c, err := s.jira.AddComment(ctx, key, commentBody)
		if err != nil {
			return errResult(err)
		}

		return jsonResult(map[string]any{
			"status":    "added",
			"commentId": c.ID,
			"issueKey":  key,
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_add_worklog",
		mcp.WithDescription("Logs time spent working on an issue."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithString("time_spent", mcp.Required(), mcp.Description("Time spent format (e.g. '2h', '1d 4h', '45m')")),
		mcp.WithString("comment", mcp.Description("Optional note describing the work completed")),
		mcp.WithString("started", mcp.Description("Optional start timestamp (ISO-8601 or Jira format, defaults to now)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		timeSpent, err := req.RequireString("time_spent")
		if err != nil {
			return errResult(err)
		}
		comment := req.GetString("comment", "")
		started := req.GetString("started", "")

		wl, err := s.jira.AddWorklog(ctx, key, timeSpent, comment, started)
		if err != nil {
			return errResult(err)
		}

		return jsonResult(map[string]any{
			"status":    "logged",
			"worklogId": wl.ID,
			"timeSpent": wl.TimeSpent,
			"issueKey":  key,
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_link_issues",
		mcp.WithDescription("Creates a relationship link between two issues (e.g. 'Blocks', 'Relates')."),
		mcp.WithString("inward_key", mcp.Required(), mcp.Description("First issue key")),
		mcp.WithString("outward_key", mcp.Required(), mcp.Description("Second issue key")),
		mcp.WithString("link_type", mcp.Required(), mcp.Description("Name of relationship (e.g. 'Blocks', 'Relates', 'Duplicate')")),
		mcp.WithString("comment", mcp.Description("Optional comment for the link")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		inward, err := req.RequireString("inward_key")
		if err != nil {
			return errResult(err)
		}
		outward, err := req.RequireString("outward_key")
		if err != nil {
			return errResult(err)
		}
		linkType, err := req.RequireString("link_type")
		if err != nil {
			return errResult(err)
		}

		if err := s.jira.CreateLink(ctx, inward, outward, linkType, req.GetString("comment", "")); err != nil {
			return errResult(err)
		}

		return textResult(fmt.Sprintf("Created '%s' link between %s and %s successfully.", linkType, inward, outward))
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_delete_issue",
		mcp.WithDescription("Permanently deletes an issue from Jira."),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}

		if err := s.jira.DeleteIssue(ctx, key); err != nil {
			return errResult(err)
		}

		if s.cache != nil {
			_ = s.cache.DeleteIssue(key)
		}

		return textResult(fmt.Sprintf("Issue %s deleted successfully.", key))
	})
}

// -----------------------------------------------------------------------------
// Agile, Sprint & Backlog Tools
// -----------------------------------------------------------------------------

func (s *Server) registerAgileTools() {
	s.mcpServer.AddTool(mcp.NewTool("jira_list_boards",
		mcp.WithDescription("Lists Jira Software agile boards (Scrum and Kanban)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("type", mcp.Description("Filter by board type: 'scrum' or 'kanban'")),
		mcp.WithInteger("limit", mcp.Description("Maximum boards to return (default 50)")),
		mcp.WithInteger("start_at", mcp.Description("Pagination start offset")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boards, err := s.jira.ListBoards(ctx, req.GetString("type", ""), req.GetInt("start_at", 0), req.GetInt("limit", 50))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(boards)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_list_sprints",
		mcp.WithDescription("Lists sprints associated with a Jira board."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithInteger("board_id", mcp.Required(), mcp.Description("Agile Board ID")),
		mcp.WithString("state", mcp.Description("Filter by state: 'active', 'future', or 'closed'")),
		mcp.WithInteger("limit", mcp.Description("Maximum sprints to return (default 50)")),
		mcp.WithInteger("start_at", mcp.Description("Pagination start offset")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := req.RequireInt("board_id")
		if err != nil {
			return errResult(err)
		}
		sprints, err := s.jira.ListSprints(ctx, boardID, req.GetString("state", ""), req.GetInt("start_at", 0), req.GetInt("limit", 50))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(sprints)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_sprint_issues",
		mcp.WithDescription("Gets all issues belonging to a specific sprint."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithInteger("sprint_id", mcp.Required(), mcp.Description("Sprint ID")),
		mcp.WithInteger("limit", mcp.Description("Maximum issues to return")),
		mcp.WithInteger("start_at", mcp.Description("Pagination start offset")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sprintID, err := req.RequireInt("sprint_id")
		if err != nil {
			return errResult(err)
		}
		issues, err := s.jira.GetSprintIssues(ctx, sprintID, req.GetInt("start_at", 0), req.GetInt("limit", 50))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(issues)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_backlog_issues",
		mcp.WithDescription("Gets all issues currently in the backlog of a board."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithInteger("board_id", mcp.Required(), mcp.Description("Agile Board ID")),
		mcp.WithInteger("limit", mcp.Description("Maximum issues to return")),
		mcp.WithInteger("start_at", mcp.Description("Pagination start offset")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := req.RequireInt("board_id")
		if err != nil {
			return errResult(err)
		}
		issues, err := s.jira.GetBacklogIssues(ctx, boardID, req.GetInt("start_at", 0), req.GetInt("limit", 50))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(issues)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_board_configuration",
		mcp.WithDescription("Retrieves the detailed column configuration, mapped issue statuses, WIP limits, and workflow type ('scrum' vs 'kanban') for an agile board."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithInteger("board_id", mcp.Required(), mcp.Description("Agile Board ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := req.RequireInt("board_id")
		if err != nil {
			return errResult(err)
		}

		cfg, err := s.jira.GetBoardConfiguration(ctx, boardID)
		if err != nil {
			return errResult(err)
		}

		if s.cache != nil {
			_ = s.cache.UpsertBoard(nil, cfg)
		}

		return jsonResult(cfg)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_board_view",
		mcp.WithDescription("Retrieves a complete visual view of an agile board (Scrum or Kanban) with all columns and backlog. Dynamically adapts to custom column workflows per board/project."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithInteger("board_id", mcp.Required(), mcp.Description("Integer Agile Board ID (use jira_list_boards to discover ID from project key)")),
		mcp.WithString("format", mcp.Enum("markdown", "json"), mcp.Description("Output format: 'markdown' (dense board layout) or 'json' (raw data). Default: 'markdown'")),
		mcp.WithBoolean("include_backlog", mcp.Description("Whether to include backlog items (default: true)")),
		mcp.WithBoolean("local_only", mcp.Description("Build view solely from local SQLite cache without hitting Jira network API (default: false)")),
		mcp.WithString("project_key", mcp.Description("Project key fallback if local_only is true (e.g. 'PROJ')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := req.RequireInt("board_id")
		if err != nil {
			return errResult(err)
		}

		format := strings.ToLower(req.GetString("format", "markdown"))
		includeBacklog := req.GetBool("include_backlog", true)
		localOnly := req.GetBool("local_only", false)
		projectKey := req.GetString("project_key", "")

		var view *jira.BoardView
		if localOnly {
			if s.cache == nil {
				return errResult(fmt.Errorf("local SQLite cache is disabled or not initialized"))
			}
			view, err = s.cache.GetLocalBoardView(boardID, projectKey, nil, includeBacklog)
			if err != nil {
				return errResult(fmt.Errorf("generating local board view: %w", err))
			}
		} else {
			view, err = s.jira.BuildBoardView(ctx, boardID, includeBacklog)
			if err != nil {
				// Fallback to local cache ONLY if board was legitimately cached before or projectKey was explicitly given
				if s.cache != nil {
					cachedCfg, _ := s.cache.GetBoard(boardID)
					if cachedCfg != nil || projectKey != "" {
						if localV, localErr := s.cache.GetLocalBoardView(boardID, projectKey, nil, includeBacklog); localErr == nil && localV.TotalCards > 0 {
							view = localV
						} else {
							return errResult(err)
						}
					} else {
						return errResult(err)
					}
				} else {
					return errResult(err)
				}
			} else if s.cache != nil {
				// Cache board info
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
		}

		if format == "json" {
			return jsonResult(view)
		}
		return textResult(view.ToMarkdown())
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_create_sprint",
		mcp.WithDescription("Creates a new sprint in an agile board."),
		mcp.WithInteger("board_id", mcp.Required(), mcp.Description("Origin Board ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Sprint name (e.g. 'Sprint 24')")),
		mcp.WithString("start_date", mcp.Description("Start date in ISO-8601 format")),
		mcp.WithString("end_date", mcp.Description("End date in ISO-8601 format")),
		mcp.WithString("goal", mcp.Description("Sprint goal")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := req.RequireInt("board_id")
		if err != nil {
			return errResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errResult(err)
		}

		sprint, err := s.jira.CreateSprint(ctx, boardID, name, req.GetString("start_date", ""), req.GetString("end_date", ""), req.GetString("goal", ""))
		if err != nil {
			return errResult(err)
		}
		return jsonResult(sprint)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_update_sprint_state",
		mcp.WithDescription("Starts or closes a sprint ('active' to start, 'closed' to complete)."),
		mcp.WithInteger("sprint_id", mcp.Required(), mcp.Description("Sprint ID")),
		mcp.WithString("state", mcp.Required(), mcp.Description("Target state: 'active' or 'closed'")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sprintID, err := req.RequireInt("sprint_id")
		if err != nil {
			return errResult(err)
		}
		state, err := req.RequireString("state")
		if err != nil {
			return errResult(err)
		}

		if err := s.jira.UpdateSprintState(ctx, sprintID, state); err != nil {
			return errResult(err)
		}
		return textResult(fmt.Sprintf("Sprint %d state updated to '%s' successfully.", sprintID, state))
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_move_issues_to_sprint",
		mcp.WithDescription("Moves one or more issues into a sprint."),
		mcp.WithInteger("sprint_id", mcp.Required(), mcp.Description("Target Sprint ID")),
		mcp.WithArray("issue_keys", mcp.Required(), mcp.Description("List of issue keys to move into the sprint")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sprintID, err := req.RequireInt("sprint_id")
		if err != nil {
			return errResult(err)
		}
		keys, err := req.RequireStringSlice("issue_keys")
		if err != nil {
			return errResult(err)
		}

		if err := s.jira.MoveIssuesToSprint(ctx, sprintID, keys); err != nil {
			return errResult(err)
		}
		return textResult(fmt.Sprintf("Moved %d issues into sprint %d successfully.", len(keys), sprintID))
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_move_issues_to_backlog",
		mcp.WithDescription("Moves one or more issues back to the board backlog."),
		mcp.WithArray("issue_keys", mcp.Required(), mcp.Description("List of issue keys to move back to the backlog")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		keys, err := req.RequireStringSlice("issue_keys")
		if err != nil {
			return errResult(err)
		}

		if err := s.jira.MoveIssuesToBacklog(ctx, keys); err != nil {
			return errResult(err)
		}
		return textResult(fmt.Sprintf("Moved %d issues back to backlog successfully.", len(keys)))
	})
}

// -----------------------------------------------------------------------------
// Analysis Tools
// -----------------------------------------------------------------------------

func (s *Server) registerAnalysisTools() {
	s.mcpServer.AddTool(mcp.NewTool("jira_analyze_backlog",
		mcp.WithDescription("Analyzes backlog health, stale issues, unassigned tickets, priority and type breakdowns."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithInteger("board_id", mcp.Description("Board ID to analyze")),
		mcp.WithString("project_key", mcp.Description("Project Key (used if board_id not provided)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID := req.GetInt("board_id", 0)
		projectKey := req.GetString("project_key", "")

		analysis, err := s.jira.AnalyzeBacklog(ctx, boardID, projectKey)
		if err != nil {
			return errResult(err)
		}

		return textResult(analysis.FormatAnalysisSummary())
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_analyze_sprint",
		mcp.WithDescription("Analyzes sprint progress, completion percentage, bottlenecks, and workload distribution by assignee."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithInteger("sprint_id", mcp.Required(), mcp.Description("Sprint ID to analyze")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sprintID, err := req.RequireInt("sprint_id")
		if err != nil {
			return errResult(err)
		}

		analysis, err := s.jira.AnalyzeSprint(ctx, sprintID)
		if err != nil {
			return errResult(err)
		}

		return textResult(analysis.FormatSprintSummary())
	})
}

// -----------------------------------------------------------------------------
// Bulk Tools
// -----------------------------------------------------------------------------

func (s *Server) registerBulkTools() {
	s.mcpServer.AddTool(mcp.NewTool("jira_bulk_create_issues",
		mcp.WithDescription("Creates multiple Jira issues in batch from a JSON array."),
		mcp.WithArray("issues", mcp.Required(), mcp.Description("Array of issue objects (projectKey, issueType, summary, description, priority, assignee, labels)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		rawList, ok := args["issues"]
		if !ok {
			return errResult(fmt.Errorf("missing 'issues' array parameter"))
		}

		listBytes, err := json.Marshal(rawList)
		if err != nil {
			return errResult(fmt.Errorf("reading 'issues' list: %w", err))
		}

		var issueRequests []jira.CreateIssueRequest
		if err := json.Unmarshal(listBytes, &issueRequests); err != nil {
			return errResult(fmt.Errorf("parsing 'issues' JSON list: %w", err))
		}

		result, err := s.jira.BulkCreateIssues(ctx, issueRequests)
		if err != nil {
			return errResult(err)
		}

		if s.cache != nil && len(result.SuccessfulKeys) > 0 {
			go func(keys []string) {
				for _, k := range keys {
					if iss, errGet := s.jira.GetIssue(context.Background(), k, nil); errGet == nil {
						_ = s.cache.UpsertIssue(iss)
					}
				}
			}(result.SuccessfulKeys)
		}

		return jsonResult(result)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_bulk_update_issues",
		mcp.WithDescription("Applies field updates across multiple Jira issues at once."),
		mcp.WithArray("issue_keys", mcp.Required(), mcp.Description("List of issue keys to update")),
		mcp.WithString("priority", mcp.Description("New priority for all issues")),
		mcp.WithString("assignee", mcp.Description("New assignee for all issues")),
		mcp.WithArray("labels", mcp.Description("Replacement labels for all issues")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		keys, err := req.RequireStringSlice("issue_keys")
		if err != nil {
			return errResult(err)
		}

		updateReq := jira.UpdateIssueRequest{
			Priority: req.GetString("priority", ""),
			Assignee: req.GetString("assignee", ""),
			Labels:   req.GetStringSlice("labels", nil),
		}

		result, err := s.jira.BulkUpdateIssues(ctx, keys, updateReq)
		if err != nil {
			return errResult(err)
		}

		if s.cache != nil && len(result.SuccessfulKeys) > 0 {
			go func(kList []string) {
				for _, k := range kList {
					if iss, errGet := s.jira.GetIssue(context.Background(), k, nil); errGet == nil {
						_ = s.cache.UpsertIssue(iss)
					}
				}
			}(result.SuccessfulKeys)
		}

		return jsonResult(result)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_bulk_transition_issues",
		mcp.WithDescription("Transitions multiple issues to a destination state in batch (e.g. moving all to 'Done')."),
		mcp.WithArray("issue_keys", mcp.Required(), mcp.Description("List of issue keys to transition")),
		mcp.WithString("transition", mcp.Required(), mcp.Description("Target transition ID or state name (e.g. 'Done', 'In Progress')")),
		mcp.WithString("comment", mcp.Description("Optional comment added during transition")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		keys, err := req.RequireStringSlice("issue_keys")
		if err != nil {
			return errResult(err)
		}
		transition, err := req.RequireString("transition")
		if err != nil {
			return errResult(err)
		}

		result, err := s.jira.BulkTransitionIssues(ctx, keys, transition, req.GetString("comment", ""))
		if err != nil {
			return errResult(err)
		}

		if s.cache != nil && len(result.SuccessfulKeys) > 0 {
			go func(kList []string) {
				for _, k := range kList {
					if iss, errGet := s.jira.GetIssue(context.Background(), k, nil); errGet == nil {
						_ = s.cache.UpsertIssue(iss)
					}
				}
			}(result.SuccessfulKeys)
		}

		return jsonResult(result)
	})
}

// -----------------------------------------------------------------------------
// Attachment & Export Tools
// -----------------------------------------------------------------------------

func (s *Server) registerAttachmentTools() {
	s.mcpServer.AddTool(mcp.NewTool("jira_list_attachments",
		mcp.WithDescription("Lists attachments uploaded to an issue with file sizes and IDs."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}

		attachments, err := s.jira.ListAttachments(ctx, key)
		if err != nil {
			return errResult(err)
		}

		return jsonResult(attachments)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_download_attachment",
		mcp.WithDescription("Downloads an attachment by ID and saves it to a local file path."),
		mcp.WithString("attachment_id", mcp.Required(), mcp.Description("Attachment ID")),
		mcp.WithString("destination_path", mcp.Required(), mcp.Description("Local destination file or directory path")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attID, err := req.RequireString("attachment_id")
		if err != nil {
			return errResult(err)
		}
		destPath, err := req.RequireString("destination_path")
		if err != nil {
			return errResult(err)
		}

		saved, err := s.jira.DownloadAttachment(ctx, attID, destPath)
		if err != nil {
			return errResult(err)
		}

		return jsonResult(map[string]any{
			"status":    "downloaded",
			"filename":  saved.Filename,
			"localPath": saved.Content,
			"sizeBytes": saved.Size,
			"mimeType":  saved.MimeType,
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_upload_attachment",
		mcp.WithDescription("Uploads a local file and attaches it to a Jira issue."),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Absolute path to the local file to upload")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}
		filePath, err := req.RequireString("file_path")
		if err != nil {
			return errResult(err)
		}

		uploaded, err := s.jira.UploadAttachment(ctx, key, filePath)
		if err != nil {
			return errResult(err)
		}

		return jsonResult(map[string]any{
			"status":      "uploaded",
			"issueKey":    key,
			"attachments": uploaded,
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_export_issues",
		mcp.WithDescription("Exports Jira search results to a local file in 'json', 'csv', or 'markdown' format."),
		mcp.WithString("jql", mcp.Required(), mcp.Description("JQL query to select issues to export")),
		mcp.WithString("format", mcp.Required(), mcp.Description("Output format: 'json', 'csv', or 'markdown'")),
		mcp.WithString("destination_path", mcp.Required(), mcp.Description("Local file path where export should be saved")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		jql, err := req.RequireString("jql")
		if err != nil {
			return errResult(err)
		}
		format, err := req.RequireString("format")
		if err != nil {
			return errResult(err)
		}
		destPath, err := req.RequireString("destination_path")
		if err != nil {
			return errResult(err)
		}

		outPath, err := s.jira.ExportIssues(ctx, jql, format, destPath)
		if err != nil {
			return errResult(err)
		}

		return textResult(fmt.Sprintf("Export completed successfully. File saved at: %s", outPath))
	})
}

// -----------------------------------------------------------------------------
// Metadata, Epics, Versions & Users Tools
// -----------------------------------------------------------------------------

func (s *Server) registerMetadataTools() {
	s.mcpServer.AddTool(mcp.NewTool("jira_get_fields",
		mcp.WithDescription("Lists all Jira system and custom fields with their IDs, keys, and names (useful for discovering custom field IDs like Story Points, Epic Link, etc.)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		fields, err := s.jira.GetFields(ctx)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(fields)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_issue_types",
		mcp.WithDescription("Lists all issue types defined in Jira (e.g. Bug, Story, Task, Epic, Sub-task)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		types, err := s.jira.GetIssueTypes(ctx)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(types)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_priorities",
		mcp.WithDescription("Lists all priority levels configured in Jira (e.g. Highest, High, Medium, Low, Lowest)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		priorities, err := s.jira.GetPriorities(ctx)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(priorities)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_find_users",
		mcp.WithDescription("Searches for Jira users by display name or email to retrieve their exact accountId or username for assignments."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("query", mcp.Required(), mcp.Description("Name or email prefix to search for")),
		mcp.WithInteger("limit", mcp.Description("Maximum users to return (default 20)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return errResult(err)
		}
		limit := req.GetInt("limit", 20)

		users, err := s.jira.FindUsers(ctx, query, limit)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(users)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_epic_issues",
		mcp.WithDescription("Lists all issues/cards that belong to a specific Epic."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("epic_key", mcp.Required(), mcp.Description("The key of the Epic (e.g. 'PROJ-10')")),
		mcp.WithInteger("limit", mcp.Description("Maximum issues to return")),
		mcp.WithInteger("start_at", mcp.Description("Pagination start offset")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		epicKey, err := req.RequireString("epic_key")
		if err != nil {
			return errResult(err)
		}
		limit := req.GetInt("limit", 50)
		startAt := req.GetInt("start_at", 0)

		issues, err := s.jira.GetEpicIssues(ctx, epicKey, startAt, limit)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(issues)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_list_versions",
		mcp.WithDescription("Lists all release versions defined for a project."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("Project key (e.g. 'PROJ')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pKey, err := req.RequireString("project_key")
		if err != nil {
			return errResult(err)
		}
		versions, err := s.jira.ListVersions(ctx, pKey)
		if err != nil {
			return errResult(err)
		}
		return jsonResult(versions)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_create_version",
		mcp.WithDescription("Creates a new software version / release in a project."),
		mcp.WithString("project_key", mcp.Required(), mcp.Description("Project key")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Version name (e.g. 'v1.2.0')")),
		mcp.WithString("description", mcp.Description("Version release notes or description")),
		mcp.WithString("release_date", mcp.Description("Release date (YYYY-MM-DD)")),
		mcp.WithBoolean("released", mcp.Description("Whether the version is already released")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pKey, err := req.RequireString("project_key")
		if err != nil {
			return errResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errResult(err)
		}

		v, err := s.jira.CreateVersion(ctx, jira.CreateVersionRequest{
			Project:     pKey,
			Name:        name,
			Description: req.GetString("description", ""),
			ReleaseDate: req.GetString("release_date", ""),
			Released:    req.GetBool("released", false),
		})
		if err != nil {
			return errResult(err)
		}

		return jsonResult(v)
	})
}

// -----------------------------------------------------------------------------
// Local SQLite Cache & Incremental Sync Tools
// -----------------------------------------------------------------------------

func (s *Server) registerCacheTools() {
	s.mcpServer.AddTool(mcp.NewTool("jira_sync_cache",
		mcp.WithDescription("Synchronizes Jira issues into the local SQLite cache incrementally (fetching ONLY new or modified tickets since the last sync)."),
		mcp.WithString("project_key", mcp.Description("Optional project key to sync (e.g. 'PROJ'). If omitted, syncs across all accessible projects.")),
		mcp.WithBoolean("full_sync", mcp.Description("If true, ignores last sync timestamp and performs a full refresh.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.cache == nil {
			return errResult(fmt.Errorf("local SQLite cache is disabled or failed to initialize"))
		}

		projectKey := req.GetString("project_key", "")
		fullSync := req.GetBool("full_sync", false)

		count, lastTs, err := s.cache.SyncIncremental(ctx, s.jira, projectKey, fullSync)
		if err != nil {
			return errResult(fmt.Errorf("incremental sync error: %w", err))
		}

		return jsonResult(map[string]any{
			"status":            "synchronized",
			"projectKey":        projectKey,
			"issuesSyncedCount": count,
			"latestTimestamp":   lastTs,
			"fullSync":          fullSync,
			"databasePath":      s.cfg.CacheDBPath,
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_search_local_cache",
		mcp.WithDescription("Searches the local SQLite cache instantly (< 5ms) without touching the network. Searches summary, description, and keys with filters."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("query", mcp.Description("Keyword text to search in summary, description or issue key")),
		mcp.WithString("project_key", mcp.Description("Filter by project key (e.g. 'PROJ')")),
		mcp.WithString("status", mcp.Description("Filter by status (e.g. 'In Progress', 'Done')")),
		mcp.WithString("assignee", mcp.Description("Filter by assignee name")),
		mcp.WithString("priority", mcp.Description("Filter by priority name")),
		mcp.WithInteger("limit", mcp.Description("Maximum results to return (default 50, max 200)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.cache == nil {
			return errResult(fmt.Errorf("local SQLite cache is disabled or failed to initialize"))
		}

		query := req.GetString("query", "")
		pKey := req.GetString("project_key", "")
		status := req.GetString("status", "")
		assignee := req.GetString("assignee", "")
		priority := req.GetString("priority", "")
		limit := req.GetInt("limit", 50)

		results, err := s.cache.Search(query, pKey, status, assignee, priority, limit)
		if err != nil {
			return errResult(err)
		}

		return jsonResult(map[string]any{
			"totalFound": len(results),
			"issues":     results,
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_cached_issue",
		mcp.WithDescription("Retrieves an issue from the local SQLite cache instantly by key (e.g. 'PROJ-123')."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("Issue key (e.g. 'PROJ-123')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.cache == nil {
			return errResult(fmt.Errorf("local SQLite cache is disabled or failed to initialize"))
		}

		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}

		cached, err := s.cache.GetIssue(key)
		if err != nil {
			return errResult(err)
		}
		if cached == nil {
			return textResult(fmt.Sprintf("Issue %s is not in local cache. Run 'jira_sync_cache' or fetch via 'jira_get_issue'.", key))
		}

		return jsonResult(cached)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_cache_stats",
		mcp.WithDescription("Displays statistics about the local SQLite cache (total stored issues, breakdown per project, last sync timestamps, database file size)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.cache == nil {
			return errResult(fmt.Errorf("local SQLite cache is disabled or failed to initialize"))
		}

		stats, err := s.cache.GetStats()
		if err != nil {
			return errResult(err)
		}

		return jsonResult(stats)
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_context_search",
		mcp.WithDescription("Performs full-text semantic and contextual search over the local SQLite cache using FTS5 and BM25 ranking. Returns ranked cards with keyword-highlighted snippets without touching the network."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search terms or topic to search (e.g. 'deadlock in postgres', 'OAuth token rotation')")),
		mcp.WithString("project_key", mcp.Description("Filter by project key (e.g. 'PROJ')")),
		mcp.WithString("status", mcp.Description("Filter by status (e.g. 'In Progress')")),
		mcp.WithString("assignee", mcp.Description("Filter by assignee name")),
		mcp.WithString("priority", mcp.Description("Filter by priority")),
		mcp.WithInteger("limit", mcp.Description("Maximum results to return (default 30)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.cache == nil {
			return errResult(fmt.Errorf("local SQLite cache is disabled or failed to initialize"))
		}

		query, err := req.RequireString("query")
		if err != nil {
			return errResult(err)
		}
		pKey := req.GetString("project_key", "")
		status := req.GetString("status", "")
		assignee := req.GetString("assignee", "")
		priority := req.GetString("priority", "")
		limit := req.GetInt("limit", 30)

		matches, err := s.cache.SearchContext(query, pKey, status, assignee, priority, limit)
		if err != nil {
			return errResult(err)
		}

		return jsonResult(map[string]any{
			"totalMatches": len(matches),
			"query":        query,
			"results":      matches,
		})
	})

	s.mcpServer.AddTool(mcp.NewTool("jira_get_context",
		mcp.WithDescription("Retrieves a comprehensive operational context pack for a ticket (combines Parent/Epic, Sibling cards in same epic, Subtasks, Blockers/Dependencies, recent Comments, and Changelog). Ideal for LLM ingestion before code changes."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("issue_key", mcp.Required(), mcp.Description("The issue key (e.g. 'PROJ-123')")),
		mcp.WithString("format", mcp.Description("Output format: 'markdown' (dense executive summary for prompt context) or 'json' (raw context pack data). Default is 'markdown'.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.cache == nil {
			return errResult(fmt.Errorf("local SQLite cache is disabled or failed to initialize"))
		}

		key, err := req.RequireString("issue_key")
		if err != nil {
			return errResult(err)
		}

		format := req.GetString("format", "markdown")

		pack, err := s.cache.GetContextPack(ctx, s.jira, key)
		if err != nil {
			return errResult(err)
		}

		if strings.ToLower(format) == "json" {
			return jsonResult(pack)
		}

		return textResult(pack.ToMarkdown())
	})
}
