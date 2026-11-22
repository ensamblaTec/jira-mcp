package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerPrompts() {
	// 1. Create Story Prompt
	s.mcpServer.AddPrompt(mcp.NewPrompt("jira_create_story",
		mcp.WithPromptDescription("Interactive prompt guide to create a well-architected user story with Definition of Done (DoD)."),
		mcp.WithArgument("project_key", mcp.ArgumentDescription("Target project key (e.g. 'PROJ')"), mcp.RequiredArgument()),
		mcp.WithArgument("summary", mcp.ArgumentDescription("Summary or feature goal"), mcp.RequiredArgument()),
	), func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		projectKey := req.Params.Arguments["project_key"]
		summary := req.Params.Arguments["summary"]

		content := fmt.Sprintf(`Please create a standard user story in project '%s' for '%s'.

Use the tool 'jira_create_issue' with the following guidelines:
1. Set 'project_key': '%s'
2. Set 'issue_type': 'Feature' or 'Story'
3. Set 'summary': '%s'
4. Set 'template': 'story' (or format description with Business Goal, Technical Scope, and Definition of Done)
5. Set 'assignee': '@me' if working on it directly

Once created, review the issue key and confirm with the user.`, projectKey, summary, projectKey, summary)

		msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(content))
		return mcp.NewGetPromptResult("Guided user story creation", []mcp.PromptMessage{msg}), nil
	})

	// 2. Report Bug Prompt
	s.mcpServer.AddPrompt(mcp.NewPrompt("jira_report_bug",
		mcp.WithPromptDescription("Interactive prompt guide to report a bug with steps to reproduce, environment, and logs."),
		mcp.WithArgument("project_key", mcp.ArgumentDescription("Target project key"), mcp.RequiredArgument()),
		mcp.WithArgument("summary", mcp.ArgumentDescription("Brief bug summary"), mcp.RequiredArgument()),
	), func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		projectKey := req.Params.Arguments["project_key"]
		summary := req.Params.Arguments["summary"]

		content := fmt.Sprintf(`Please report a bug in project '%s' with title '%s'.

Use the tool 'jira_create_issue' with:
1. Set 'project_key': '%s'
2. Set 'issue_type': 'Error' or 'Bug'
3. Set 'summary': '%s'
4. Set 'template': 'bug'
5. Make sure to detail: Actual Behavior, Expected Behavior, Steps to Reproduce, and Logs/Environment.`, projectKey, summary, projectKey, summary)

		msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(content))
		return mcp.NewGetPromptResult("Guided bug reporting", []mcp.PromptMessage{msg}), nil
	})

	// 3. Breakdown Feature Prompt
	s.mcpServer.AddPrompt(mcp.NewPrompt("jira_breakdown_feature",
		mcp.WithPromptDescription("Guides the architectural breakdown of a Feature into standard engineering subtasks (Analysis, Design, Build, Test, Docs) via jira_bulk_create_issues."),
		mcp.WithArgument("parent_key", mcp.ArgumentDescription("Parent Feature/Story key (e.g. 'PROJ-34')"), mcp.RequiredArgument()),
		mcp.WithArgument("feature_title", mcp.ArgumentDescription("Feature title to break down"), mcp.RequiredArgument()),
	), func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		parentKey := req.Params.Arguments["parent_key"]
		title := req.Params.Arguments["feature_title"]

		content := fmt.Sprintf(`Decompose the parent feature '%s' (%s) into standard engineering subtasks.

Recommended subtasks to create in a single batch using 'jira_bulk_create_issues':
1. Analysis-Subtask: "Analizar requisitos funcionales y casos límite para %s"
2. Design-Subtask: "Diseñar arquitectura técnica, contratos de API y modelos de datos para %s"
3. Build-Subtask: "Implementar componentes de backend y frontend para %s"
4. Test-Subtask: "Escribir pruebas unitarias, de integración y E2E para %s"
5. Documentation-Subtask: "Documentar endpoints, flujo operativo y changelog para %s"

All subtasks must specify:
- 'parent_key': '%s'
- 'project_key': extracted from parent key prefix
- 'issue_type': 'Sub-task'
- 'assignee': '@me' (or designated engineer)

Execute 'jira_bulk_create_issues' with this validated plan to prevent partial failures.`, parentKey, title, title, title, title, title, title, parentKey)

		msg := mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(content))
		return mcp.NewGetPromptResult("Standard Feature engineering breakdown", []mcp.PromptMessage{msg}), nil
	})
}
