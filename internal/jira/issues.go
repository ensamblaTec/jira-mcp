package jira

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// GetMyself returns details of the currently authenticated user.
func (c *Client) GetMyself(ctx context.Context) (*User, error) {
	path := c.ApiPath("myself")
	var user User
	if err := c.Get(ctx, path, nil, &user); err != nil {
		return nil, fmt.Errorf("getting current user info: %w", err)
	}
	return &user, nil
}

// ListProjects returns all accessible projects.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	path := c.ApiPath("project")
	var projects []Project
	if err := c.Get(ctx, path, nil, &projects); err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	return projects, nil
}

// GetProject returns details for a specific project by key or ID.
func (c *Client) GetProject(ctx context.Context, projectKeyOrID string) (*Project, error) {
	path := c.ApiPath(fmt.Sprintf("project/%s", projectKeyOrID))
	var project Project
	if err := c.Get(ctx, path, nil, &project); err != nil {
		return nil, fmt.Errorf("getting project '%s': %w", projectKeyOrID, err)
	}
	return &project, nil
}

// GetIssue retrieves an issue by key (e.g. PROJ-123) with specified fields.
func (c *Client) GetIssue(ctx context.Context, issueKey string, fields []string) (*Issue, error) {
	path := c.ApiPath(fmt.Sprintf("issue/%s", issueKey))
	q := url.Values{}
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	} else {
		q.Set("fields", "*all")
	}
	q.Set("expand", "changelog,transitions,renderedFields")

	var issue Issue
	if err := c.Get(ctx, path, q, &issue); err != nil {
		return nil, fmt.Errorf("getting issue '%s': %w", issueKey, err)
	}
	return &issue, nil
}

// SearchIssues performs a JQL search with pagination, supporting modern Jira Cloud /search/jql and legacy /search.
func (c *Client) SearchIssues(ctx context.Context, jql string, fields []string, startAt, maxResults int, nextPageToken ...string) (*SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 50
	}
	if maxResults > 100 {
		maxResults = 100
	}

	cleanJQL := strings.TrimSpace(jql)
	if cleanJQL == "" {
		cleanJQL = "id is not null ORDER BY created DESC"
	} else if strings.HasPrefix(strings.ToLower(cleanJQL), "order by") {
		cleanJQL = "id is not null " + cleanJQL
	}

	cloudPayload := map[string]any{
		"jql":        cleanJQL,
		"maxResults": maxResults,
	}
	if len(nextPageToken) > 0 && nextPageToken[0] != "" {
		cloudPayload["nextPageToken"] = nextPageToken[0]
	}

	serverPayload := map[string]any{
		"jql":        cleanJQL,
		"startAt":    startAt,
		"maxResults": maxResults,
	}
	if len(fields) > 0 {
		cloudPayload["fields"] = fields
		serverPayload["fields"] = fields
	} else {
		defaultFields := []string{
			"summary", "status", "priority", "issuetype", "assignee", "reporter",
			"created", "updated", "labels", "components", "fixVersions", "duedate",
		}
		cloudPayload["fields"] = defaultFields
		serverPayload["fields"] = defaultFields
	}

	var result SearchResult
	var lastErr error

	// If startAt > 0 and no nextPageToken was provided, skip Cloud /search/jql
	// because Cloud v3 search/jql doesn't accept startAt.
	hasToken := len(nextPageToken) > 0 && nextPageToken[0] != ""
	if startAt == 0 || hasToken {
		cloudJQLPath := "/rest/api/3/search/jql"
		if errCloud := c.Post(ctx, cloudJQLPath, cloudPayload, &result); errCloud == nil {
			return &result, nil
		} else {
			lastErr = errCloud
		}
	}

	// Fallback to configured ApiPath("search") for Server / Data Center or v2
	standardPath := c.ApiPath("search")
	if errStandard := c.Post(ctx, standardPath, serverPayload, &result); errStandard == nil {
		return &result, nil
	} else {
		lastErr = errStandard
	}

	// Fallback to GET on standard path
	q := url.Values{
		"jql":        []string{cleanJQL},
		"startAt":    []string{strconv.Itoa(startAt)},
		"maxResults": []string{strconv.Itoa(maxResults)},
	}
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	}
	if errGet := c.Get(ctx, standardPath, q, &result); errGet == nil {
		return &result, nil
	} else {
		lastErr = errGet
	}

	return nil, fmt.Errorf("searching issues with JQL '%s': %w", cleanJQL, lastErr)
}

// TextToADF converts plain or markdown text into Atlassian Document Format (ADF) v3.
// Paragraphs separated by blank lines are preserved as distinct ADF paragraphs,
// and single newlines are represented using hardBreak nodes.
func TextToADF(text string) map[string]any {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	rawParagraphs := strings.Split(text, "\n\n")
	var paragraphs []any

	for _, p := range rawParagraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lines := strings.Split(p, "\n")
		var pContent []any
		for i, line := range lines {
			if i > 0 {
				pContent = append(pContent, map[string]any{
					"type": "hardBreak",
				})
			}
			if line != "" {
				pContent = append(pContent, map[string]any{
					"type": "text",
					"text": line,
				})
			}
		}
		if len(pContent) > 0 {
			paragraphs = append(paragraphs, map[string]any{
				"type":    "paragraph",
				"content": pContent,
			})
		}
	}

	if len(paragraphs) == 0 {
		if strings.TrimSpace(text) != "" {
			paragraphs = append(paragraphs, map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": text,
					},
				},
			})
		} else {
			paragraphs = append(paragraphs, map[string]any{
				"type": "paragraph",
			})
		}
	}

	return map[string]any{
		"version": 1,
		"type":    "doc",
		"content": paragraphs,
	}
}

// SanitizeLabel ensures Jira label compliance (no whitespace, trimmed).
func SanitizeLabel(l string) string {
	l = strings.TrimSpace(l)
	l = strings.ReplaceAll(l, " ", "-")
	return l
}

// GetIssueTemplate returns standard engineering templates for user stories, bug reports, and spikes.
func GetIssueTemplate(templateType string) string {
	switch strings.ToLower(strings.TrimSpace(templateType)) {
	case "story", "historia", "feature":
		return `## Objetivo de Negocio
Como [rol/usuario], quiero [acción/funcionalidad] para [beneficio/valor esperado].

## Alcance Técnico
- Componentes y módulos impactados:
- Criterios de diseño/arquitectura:

## Criterios de Aceptación (Definition of Done)
- [ ] Implementación de funcionalidad completada
- [ ] Pruebas unitarias e integración con cobertura adecuada
- [ ] Validación de escenarios de error y casos límite
- [ ] Documentación técnica y logs operativos actualizados`

	case "bug", "error", "defecto":
		return `## Comportamiento Actual
Descripción clara y concisa del error detectado.

## Comportamiento Esperado
Qué comportamiento se esperaba según especificación.

## Pasos para Reproducir
1. 
2. 
3. 

## Entorno & Evidencias
- Entorno: (Local / Staging / Prod)
- Logs / Stack Trace:
- Causa raíz preliminar (si se conoce):`

	case "spike", "investigacion", "research":
		return `## Hipótesis / Pregunta Clave
Qué interrogante técnica o riesgo de arquitectura se pretende responder o des-arriesgar.

## Timebox
Tiempo límite asignado para la investigación.

## Entregables Esperados
- [ ] Documento de hallazgos y recomendación técnica
- [ ] PoC / Prototipo de validación técnica
- [ ] Definición de historias/tareas derivadas`

	default:
		return ""
	}
}

// CreateIssue creates a new issue or subtask.
func (c *Client) CreateIssue(ctx context.Context, req CreateIssueRequest) (*Issue, error) {
	path := c.ApiPath("issue")

	fields := make(map[string]any)
	fields["project"] = map[string]string{"key": req.ProjectKey}
	fields["issuetype"] = map[string]string{"name": req.IssueType}
	fields["summary"] = req.Summary

	if req.Description == "" && req.Template != "" {
		req.Description = GetIssueTemplate(req.Template)
	}

	if req.Description != "" {
		if c.apiVersion == "3" {
			fields["description"] = TextToADF(req.Description)
		} else {
			fields["description"] = req.Description
		}
	}

	if req.Priority != "" {
		fields["priority"] = map[string]string{"name": req.Priority}
	}

	if req.Assignee != "" {
		resolvedAssignee, err := c.ResolveAssignee(ctx, req.Assignee)
		if err == nil && resolvedAssignee != "" {
			req.Assignee = resolvedAssignee
		}
		if c.apiVersion == "3" || strings.Contains(req.Assignee, ":") || len(req.Assignee) >= 24 {
			fields["assignee"] = map[string]string{"accountId": req.Assignee}
		} else {
			fields["assignee"] = map[string]string{"name": req.Assignee}
		}
	}

	if len(req.Labels) > 0 {
		var cleanLabels []string
		for _, l := range req.Labels {
			if cl := SanitizeLabel(l); cl != "" {
				cleanLabels = append(cleanLabels, cl)
			}
		}
		if len(cleanLabels) > 0 {
			fields["labels"] = cleanLabels
		}
	}

	if len(req.Components) > 0 {
		comps := make([]map[string]string, len(req.Components))
		for i, comp := range req.Components {
			comps[i] = map[string]string{"name": comp}
		}
		fields["components"] = comps
	}

	if len(req.FixVersions) > 0 {
		versions := make([]map[string]string, len(req.FixVersions))
		for i, v := range req.FixVersions {
			versions[i] = map[string]string{"name": v}
		}
		fields["fixVersions"] = versions
	}

	if req.DueDate != "" {
		fields["duedate"] = req.DueDate
	}

	if req.ParentKey != "" {
		fields["parent"] = map[string]string{"key": req.ParentKey}
	}

	// Merge any extra custom fields
	for k, v := range req.CustomFields {
		fields[k] = v
	}

	payload := map[string]any{"fields": fields}
	var created Issue
	if err := c.Post(ctx, path, payload, &created); err != nil {
		return nil, fmt.Errorf("creating issue: %w", err)
	}

	return &created, nil
}

// UpdateIssue updates fields on an existing issue.
func (c *Client) UpdateIssue(ctx context.Context, issueKey string, req UpdateIssueRequest) error {
	path := c.ApiPath(fmt.Sprintf("issue/%s", issueKey))

	fields := make(map[string]any)
	update := make(map[string]any)

	if req.Summary != "" {
		fields["summary"] = req.Summary
	}
	if req.Description != "" {
		if c.apiVersion == "3" {
			fields["description"] = TextToADF(req.Description)
		} else {
			fields["description"] = req.Description
		}
	}
	if req.Priority != "" {
		fields["priority"] = map[string]string{"name": req.Priority}
	}
	if req.Assignee != "" {
		if req.Assignee == "unassigned" || req.Assignee == "none" {
			fields["assignee"] = nil
		} else {
			resolvedAssignee, err := c.ResolveAssignee(ctx, req.Assignee)
			if err == nil && resolvedAssignee != "" {
				req.Assignee = resolvedAssignee
			}
			if c.apiVersion == "3" || strings.Contains(req.Assignee, ":") || len(req.Assignee) >= 24 {
				fields["assignee"] = map[string]string{"accountId": req.Assignee}
			} else {
				fields["assignee"] = map[string]string{"name": req.Assignee}
			}
		}
	}

	// Labels: Full replacement takes precedence over atomic add/remove
	if req.Labels != nil {
		var cleanLabels []string
		for _, l := range req.Labels {
			if cl := SanitizeLabel(l); cl != "" {
				cleanLabels = append(cleanLabels, cl)
			}
		}
		fields["labels"] = cleanLabels
	} else if len(req.AddLabels) > 0 || len(req.RemoveLabels) > 0 {
		var labelOps []map[string]string
		for _, al := range req.AddLabels {
			if cl := SanitizeLabel(al); cl != "" {
				labelOps = append(labelOps, map[string]string{"add": cl})
			}
		}
		for _, rl := range req.RemoveLabels {
			if cl := SanitizeLabel(rl); cl != "" {
				labelOps = append(labelOps, map[string]string{"remove": cl})
			}
		}
		if len(labelOps) > 0 {
			update["labels"] = labelOps
		}
	}

	if req.Components != nil {
		comps := make([]map[string]string, len(req.Components))
		for i, comp := range req.Components {
			comps[i] = map[string]string{"name": comp}
		}
		fields["components"] = comps
	}
	if req.FixVersions != nil {
		versions := make([]map[string]string, len(req.FixVersions))
		for i, v := range req.FixVersions {
			versions[i] = map[string]string{"name": v}
		}
		fields["fixVersions"] = versions
	}
	if req.DueDate != "" {
		fields["duedate"] = req.DueDate
	}

	for k, v := range req.CustomFields {
		fields[k] = v
	}

	payload := make(map[string]any)
	if len(fields) > 0 {
		payload["fields"] = fields
	}
	if len(update) > 0 {
		payload["update"] = update
	}

	if len(payload) == 0 {
		return fmt.Errorf("no fields or updates provided for issue '%s'", issueKey)
	}

	if err := c.Put(ctx, path, payload, nil); err != nil {
		return fmt.Errorf("updating issue '%s': %w", issueKey, err)
	}

	return nil
}

// DeleteIssue deletes an issue by key.
func (c *Client) DeleteIssue(ctx context.Context, issueKey string) error {
	path := c.ApiPath(fmt.Sprintf("issue/%s", issueKey))
	if err := c.Delete(ctx, path); err != nil {
		return fmt.Errorf("deleting issue '%s': %w", issueKey, err)
	}
	return nil
}

// AssignIssue assigns an issue to a user by accountId or username (or unassigns if empty).
func (c *Client) AssignIssue(ctx context.Context, issueKey string, accountIDOrName string) error {
	path := c.ApiPath(fmt.Sprintf("issue/%s/assignee", issueKey))

	accountIDOrName = strings.TrimSpace(accountIDOrName)
	if accountIDOrName != "" && accountIDOrName != "unassigned" && accountIDOrName != "none" {
		if resolved, err := c.ResolveAssignee(ctx, accountIDOrName); err == nil && resolved != "" {
			accountIDOrName = resolved
		}
	}

	var payload map[string]any
	if accountIDOrName == "" || accountIDOrName == "unassigned" || accountIDOrName == "none" {
		if c.apiVersion == "3" {
			payload = map[string]any{"accountId": nil}
		} else {
			payload = map[string]any{"name": nil}
		}
	} else if c.apiVersion == "3" || strings.Contains(accountIDOrName, ":") || len(accountIDOrName) >= 24 {
		payload = map[string]any{"accountId": accountIDOrName}
	} else {
		payload = map[string]any{"name": accountIDOrName}
	}

	if err := c.Put(ctx, path, payload, nil); err != nil {
		return fmt.Errorf("assigning issue '%s': %w", issueKey, err)
	}
	return nil
}

// GetTransitions returns available transitions for an issue.
func (c *Client) GetTransitions(ctx context.Context, issueKey string) ([]Transition, error) {
	path := c.ApiPath(fmt.Sprintf("issue/%s/transitions", issueKey))
	var resp TransitionsResponse
	if err := c.Get(ctx, path, nil, &resp); err != nil {
		return nil, fmt.Errorf("getting transitions for '%s': %w", issueKey, err)
	}
	return resp.Transitions, nil
}

// TransitionIssue executes a workflow transition on an issue.
// transitionIDOrName can be a numeric ID (e.g. "21") or a transition/status name (e.g. "DEPLOY", "Done").
// Optional fields can be passed to satisfy transition screen requirements (e.g. resolution, fixVersions).
func (c *Client) TransitionIssue(ctx context.Context, issueKey string, transitionIDOrName string, comment string, fields ...map[string]any) error {
	path := c.ApiPath(fmt.Sprintf("issue/%s/transitions", issueKey))

	targetID := strings.TrimSpace(transitionIDOrName)
	if _, err := strconv.Atoi(targetID); err != nil {
		// Non-numeric: resolve name against live available transitions
		available, err := c.GetTransitions(ctx, issueKey)
		if err != nil {
			return fmt.Errorf("fetching available transitions for '%s' to resolve '%s': %w", issueKey, targetID, err)
		}

		var matches []Transition
		targetLower := strings.ToLower(targetID)
		for _, t := range available {
			if strings.EqualFold(t.Name, targetID) || (t.To != nil && strings.EqualFold(t.To.Name, targetID)) {
				matches = append(matches, t)
			}
		}

		// If no exact match, try prefix/contains
		if len(matches) == 0 {
			for _, t := range available {
				if strings.Contains(strings.ToLower(t.Name), targetLower) || (t.To != nil && strings.Contains(strings.ToLower(t.To.Name), targetLower)) {
					matches = append(matches, t)
				}
			}
		}

		if len(matches) == 1 {
			targetID = matches[0].ID
		} else if len(matches) == 0 {
			var availDesc []string
			for _, t := range available {
				toName := "unknown"
				if t.To != nil {
					toName = t.To.Name
				}
				availDesc = append(availDesc, fmt.Sprintf("[ID: %s] '%s' -> '%s'", t.ID, t.Name, toName))
			}
			return fmt.Errorf("transition '%s' not found for issue %s; available transitions: %s", targetID, issueKey, strings.Join(availDesc, ", "))
		} else {
			var ambigDesc []string
			for _, m := range matches {
				toName := "unknown"
				if m.To != nil {
					toName = m.To.Name
				}
				ambigDesc = append(ambigDesc, fmt.Sprintf("[ID: %s] '%s' -> '%s'", m.ID, m.Name, toName))
			}
			return fmt.Errorf("transition '%s' is ambiguous for issue %s (multiple matches: %s); please specify the exact transition ID", targetID, issueKey, strings.Join(ambigDesc, ", "))
		}
	}

	payload := map[string]any{
		"transition": map[string]string{"id": targetID},
	}

	if len(fields) > 0 && fields[0] != nil && len(fields[0]) > 0 {
		payload["fields"] = fields[0]
	}

	if comment != "" {
		if c.apiVersion == "3" {
			payload["update"] = map[string]any{
				"comment": []any{
					map[string]any{
						"add": map[string]any{
							"body": TextToADF(comment),
						},
					},
				},
			}
		} else {
			payload["update"] = map[string]any{
				"comment": []any{
					map[string]any{
						"add": map[string]any{"body": comment},
					},
				},
			}
		}
	}

	if err := c.Post(ctx, path, payload, nil); err != nil {
		return fmt.Errorf("transitioning issue '%s' to transition '%s': %w", issueKey, targetID, err)
	}

	return nil
}

// GetChangelog retrieves the history of changes for an issue with optional limit (default 25, max 50).
func (c *Client) GetChangelog(ctx context.Context, issueKey string, limit ...int) (*ChangelogResponse, error) {
	maxResults := 25
	if len(limit) > 0 && limit[0] > 0 {
		maxResults = limit[0]
		if maxResults > 50 {
			maxResults = 50
		}
	}
	path := c.ApiPath(fmt.Sprintf("issue/%s/changelog", issueKey))
	q := url.Values{}
	q.Set("maxResults", strconv.Itoa(maxResults))
	var resp ChangelogResponse
	if err := c.Get(ctx, path, q, &resp); err != nil {
		return nil, fmt.Errorf("getting changelog for '%s': %w", issueKey, err)
	}
	if len(resp.Histories) > maxResults {
		resp.Histories = resp.Histories[:maxResults]
	}
	return &resp, nil
}

// GetFields retrieves all system and custom fields available in Jira.
func (c *Client) GetFields(ctx context.Context) ([]Field, error) {
	path := c.ApiPath("field")
	var fields []Field
	if err := c.Get(ctx, path, nil, &fields); err != nil {
		return nil, fmt.Errorf("getting jira fields: %w", err)
	}
	return fields, nil
}

// GetIssueTypes retrieves all issue types defined in Jira.
func (c *Client) GetIssueTypes(ctx context.Context) ([]IssueType, error) {
	path := c.ApiPath("issuetype")
	var types []IssueType
	if err := c.Get(ctx, path, nil, &types); err != nil {
		return nil, fmt.Errorf("getting issue types: %w", err)
	}
	return types, nil
}

// GetPriorities retrieves all priorities defined in Jira.
func (c *Client) GetPriorities(ctx context.Context) ([]Priority, error) {
	path := c.ApiPath("priority")
	var priorities []Priority
	if err := c.Get(ctx, path, nil, &priorities); err != nil {
		return nil, fmt.Errorf("getting priorities: %w", err)
	}
	return priorities, nil
}

// FindUsers searches for users by query string (displayName or email).
func (c *Client) FindUsers(ctx context.Context, query string, maxResults int) ([]User, error) {
	path := c.ApiPath("user/search")
	if maxResults <= 0 {
		maxResults = 20
	}

	q := url.Values{
		"query":      []string{query},
		"maxResults": []string{strconv.Itoa(maxResults)},
	}

	var users []User
	if err := c.Get(ctx, path, q, &users); err != nil {
		// Fallback for Data Center / Server using username param
		qServer := url.Values{
			"username":   []string{query},
			"maxResults": []string{strconv.Itoa(maxResults)},
		}
		if errServer := c.Get(ctx, path, qServer, &users); errServer != nil {
			return nil, fmt.Errorf("searching users for '%s': %w", query, err)
		}
	}

	return users, nil
}

// GetEpicIssues retrieves all child issues linked to an epic.
func (c *Client) GetEpicIssues(ctx context.Context, epicKey string, startAt, maxResults int) (*SearchResult, error) {
	jql := fmt.Sprintf("parent = '%s' OR \"Epic Link\" = '%s' ORDER BY rank ASC, created ASC", epicKey, epicKey)
	return c.SearchIssues(ctx, jql, nil, startAt, maxResults)
}

// ListVersions returns all releases/versions for a project.
func (c *Client) ListVersions(ctx context.Context, projectKey string) ([]Version, error) {
	path := c.ApiPath(fmt.Sprintf("project/%s/versions", projectKey))
	var versions []Version
	if err := c.Get(ctx, path, nil, &versions); err != nil {
		return nil, fmt.Errorf("listing versions for project '%s': %w", projectKey, err)
	}
	return versions, nil
}

// CreateVersion creates a new release/version in a project.
func (c *Client) CreateVersion(ctx context.Context, req CreateVersionRequest) (*Version, error) {
	path := c.ApiPath("version")
	var created Version
	if err := c.Post(ctx, path, req, &created); err != nil {
		return nil, fmt.Errorf("creating version '%s' in project '%s': %w", req.Name, req.Project, err)
	}
	return &created, nil
}
