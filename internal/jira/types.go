package jira

import (
	"encoding/json"
	"time"
)

// User represents a Jira user profile.
type User struct {
	AccountID    string `json:"accountId,omitempty"`
	Name         string `json:"name,omitempty"`
	Key          string `json:"key,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	Active       bool   `json:"active,omitempty"`
	TimeZone     string `json:"timeZone,omitempty"`
}

// Project represents a Jira project.
type Project struct {
	ID          string      `json:"id,omitempty"`
	Key         string      `json:"key,omitempty"`
	Name        string      `json:"name,omitempty"`
	ProjectType string      `json:"projectTypeKey,omitempty"`
	Lead        *User       `json:"lead,omitempty"`
	IssueTypes  []IssueType `json:"issueTypes,omitempty"`
	Components  []Component `json:"components,omitempty"`
	Versions    []Version   `json:"versions,omitempty"`
}

// IssueType represents an issue type (Story, Task, Bug, Epic, Sub-task).
type IssueType struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Subtask     bool   `json:"subtask,omitempty"`
	IconURL     string `json:"iconUrl,omitempty"`
}

// Priority represents an issue priority (Highest, High, Medium, Low, Lowest).
type Priority struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// StatusCategory represents the high-level category of a status (To Do, In Progress, Done).
type StatusCategory struct {
	ID   int    `json:"id,omitempty"`
	Key  string `json:"key,omitempty"`
	Name string `json:"name,omitempty"`
}

// Status represents the workflow status of an issue.
type Status struct {
	ID             string          `json:"id,omitempty"`
	Name           string          `json:"name,omitempty"`
	Description    string          `json:"description,omitempty"`
	StatusCategory *StatusCategory `json:"statusCategory,omitempty"`
}

// Component represents a project component.
type Component struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// Field represents metadata of a Jira system or custom field.
type Field struct {
	ID          string   `json:"id"`
	Key         string   `json:"key,omitempty"`
	Name        string   `json:"name"`
	Custom      bool     `json:"custom"`
	Navigable   bool     `json:"navigable"`
	Searchable  bool     `json:"searchable"`
	ClauseNames []string `json:"clauseNames,omitempty"`
}

// CreateVersionRequest payload to create a release/version.
type CreateVersionRequest struct {
	Project     string `json:"project"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
}

// Version represents a project release / fixVersion.
type Version struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Archived    bool   `json:"archived,omitempty"`
	Released    bool   `json:"released,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
}

// TimeTracking represents spent/remaining estimate.
type TimeTracking struct {
	OriginalEstimate         string `json:"originalEstimate,omitempty"`
	RemainingEstimate        string `json:"remainingEstimate,omitempty"`
	TimeSpent                string `json:"timeSpent,omitempty"`
	OriginalEstimateSeconds  int64  `json:"originalEstimateSeconds,omitempty"`
	RemainingEstimateSeconds int64  `json:"remainingEstimateSeconds,omitempty"`
	TimeSpentSeconds         int64  `json:"timeSpentSeconds,omitempty"`
}

// Attachment represents an uploaded file metadata.
type Attachment struct {
	ID        string    `json:"id,omitempty"`
	Filename  string    `json:"filename,omitempty"`
	Author    *User     `json:"author,omitempty"`
	Created   time.Time `json:"created,omitempty"`
	Size      int64     `json:"size,omitempty"`
	MimeType  string    `json:"mimeType,omitempty"`
	Content   string    `json:"content,omitempty"`
	Thumbnail string    `json:"thumbnail,omitempty"`
}

// IssueFields contains all standard fields of a Jira issue.
type IssueFields struct {
	Summary      string        `json:"summary,omitempty"`
	Description  any           `json:"description,omitempty"` // String (API v2) or ADF Object (API v3)
	IssueType    *IssueType    `json:"issuetype,omitempty"`
	Project      *Project      `json:"project,omitempty"`
	Priority     *Priority     `json:"priority,omitempty"`
	Status       *Status       `json:"status,omitempty"`
	Assignee     *User         `json:"assignee,omitempty"`
	Reporter     *User         `json:"reporter,omitempty"`
	Labels       []string      `json:"labels,omitempty"`
	Components   []Component   `json:"components,omitempty"`
	FixVersions  []Version     `json:"fixVersions,omitempty"`
	Created      string        `json:"created,omitempty"`
	Updated      string        `json:"updated,omitempty"`
	DueDate      string        `json:"duedate,omitempty"`
	Parent       *Issue        `json:"parent,omitempty"`
	Subtasks     []Issue       `json:"subtasks,omitempty"`
	IssueLinks   []IssueLink   `json:"issuelinks,omitempty"`
	Attachment   []Attachment  `json:"attachment,omitempty"`
	TimeTracking *TimeTracking `json:"timetracking,omitempty"`

	// Unknown/Custom fields (e.g. Story Points, Sprint, Epic Link)
	CustomFields map[string]any `json:"-"`
}

// Issue represents a full Jira issue.
type Issue struct {
	ID     string      `json:"id,omitempty"`
	Key    string      `json:"key,omitempty"`
	Self   string      `json:"self,omitempty"`
	Fields IssueFields `json:"fields,omitempty"`
}

// SearchResult represents the response of /rest/api/2/search, /rest/api/3/search/jql or JQL query.
type SearchResult struct {
	StartAt       int     `json:"startAt,omitempty"`
	MaxResults    int     `json:"maxResults,omitempty"`
	Total         int     `json:"total,omitempty"`
	IsLast        bool    `json:"isLast,omitempty"`
	NextPageToken string  `json:"nextPageToken,omitempty"`
	Issues        []Issue `json:"issues"`
}

// Comment represents a comment on an issue.
type Comment struct {
	ID      string `json:"id,omitempty"`
	Author  *User  `json:"author,omitempty"`
	Body    any    `json:"body,omitempty"` // String or ADF
	Created string `json:"created,omitempty"`
	Updated string `json:"updated,omitempty"`
}

// CommentsResponse represents the list of comments on an issue.
type CommentsResponse struct {
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
	Comments   []Comment `json:"comments"`
}

// Transition represents a valid workflow transition.
type Transition struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	To   *Status `json:"to,omitempty"`
}

// TransitionsResponse represents available transitions for an issue.
type TransitionsResponse struct {
	Transitions []Transition `json:"transitions"`
}

// Worklog represents a worklog entry.
type Worklog struct {
	ID               string `json:"id,omitempty"`
	Author           *User  `json:"author,omitempty"`
	Comment          any    `json:"comment,omitempty"`
	Started          string `json:"started,omitempty"`
	TimeSpent        string `json:"timeSpent,omitempty"`
	TimeSpentSeconds int64  `json:"timeSpentSeconds,omitempty"`
}

// WorklogsResponse represents worklogs on an issue.
type WorklogsResponse struct {
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
	Worklogs   []Worklog `json:"worklogs"`
}

// IssueLinkType represents the type of relation between issues.
type IssueLinkType struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Inward  string `json:"inward,omitempty"`
	Outward string `json:"outward,omitempty"`
}

// IssueLink represents an existing link between issues.
type IssueLink struct {
	ID           string         `json:"id,omitempty"`
	Type         *IssueLinkType `json:"type,omitempty"`
	InwardIssue  *Issue         `json:"inwardIssue,omitempty"`
	OutwardIssue *Issue         `json:"outwardIssue,omitempty"`
}

// IssueLinkTypesResponse represents the available link types.
type IssueLinkTypesResponse struct {
	IssueLinkTypes []IssueLinkType `json:"issueLinkTypes"`
}

// Board represents a Jira Agile Board (Scrum or Kanban).
type Board struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "scrum" or "kanban"
}

// BoardColumnStatus models an issue status mapped to a board column.
type BoardColumnStatus struct {
	ID   string `json:"id"`
	Self string `json:"self,omitempty"`
}

// BoardColumn models a column in a Jira board.
type BoardColumn struct {
	Name     string              `json:"name"`
	Statuses []BoardColumnStatus `json:"statuses"`
	Min      int                 `json:"min,omitempty"`
	Max      int                 `json:"max,omitempty"`
}

// ColumnConfig models the column configuration of a board.
type ColumnConfig struct {
	Columns        []BoardColumn `json:"columns"`
	ConstraintType string        `json:"constraintType,omitempty"`
}

// BoardLocation models project / space context for a board.
type BoardLocation struct {
	Type        string `json:"type,omitempty"`
	ProjectKey  string `json:"projectKey,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// BoardConfiguration models the full configuration of an agile board.
type BoardConfiguration struct {
	ID           int            `json:"id"`
	Name         string         `json:"name"`
	Type         string         `json:"type"` // "scrum" or "kanban"
	Location     *BoardLocation `json:"location,omitempty"`
	ColumnConfig ColumnConfig   `json:"columnConfig"`
}

// BoardCard represents a single issue formatted for a board column.
type BoardCard struct {
	Key            string   `json:"key"`
	Summary        string   `json:"summary"`
	IssueType      string   `json:"issueType"`
	Status         string   `json:"status"`
	StatusCategory string   `json:"statusCategory,omitempty"`
	Priority       string   `json:"priority"`
	AssigneeName   string   `json:"assigneeName"`
	AssigneeID     string   `json:"assigneeId,omitempty"`
	ParentKey      string   `json:"parentKey,omitempty"`
	EpicKey        string   `json:"epicKey,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	Components     []string `json:"components,omitempty"`
	UpdatedAt      string   `json:"updatedAt,omitempty"`
}

// BoardColumnView holds cards belonging to a specific board column.
type BoardColumnView struct {
	Name  string      `json:"name"`
	Cards []BoardCard `json:"cards"`
	Min   int         `json:"min,omitempty"`
	Max   int         `json:"max,omitempty"`
}

// BoardView represents a fully populated agile board view with its columns and backlog.
type BoardView struct {
	BoardID       int               `json:"boardId"`
	BoardName     string            `json:"boardName"`
	BoardType     string            `json:"boardType"` // "scrum" or "kanban"
	ProjectKey    string            `json:"projectKey"`
	ActiveSprint  *Sprint           `json:"activeSprint,omitempty"`
	ActiveSprints []Sprint          `json:"activeSprints,omitempty"`
	Columns       []BoardColumnView `json:"columns"`
	Backlog       []BoardCard       `json:"backlog"`
	TotalCards    int               `json:"totalCards"`
}

// BoardsResponse represents a list of agile boards.
type BoardsResponse struct {
	MaxResults int     `json:"maxResults"`
	StartAt    int     `json:"startAt"`
	Total      int     `json:"total"`
	IsLast     bool    `json:"isLast"`
	Values     []Board `json:"values"`
}

// Sprint represents a Jira sprint.
type Sprint struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	State         string `json:"state"` // "active", "future", "closed"
	StartDate     string `json:"startDate,omitempty"`
	EndDate       string `json:"endDate,omitempty"`
	CompleteDate  string `json:"completeDate,omitempty"`
	OriginBoardID int    `json:"originBoardId,omitempty"`
	Goal          string `json:"goal,omitempty"`
}

// SprintsResponse represents a list of sprints.
type SprintsResponse struct {
	MaxResults int      `json:"maxResults"`
	StartAt    int      `json:"startAt"`
	Total      int      `json:"total"`
	IsLast     bool     `json:"isLast"`
	Values     []Sprint `json:"values"`
}

// ChangelogItem represents a single field change in history.
type ChangelogItem struct {
	Field      string `json:"field"`
	FieldType  string `json:"fieldType"`
	FromString string `json:"fromString"`
	ToString   string `json:"toString"`
}

// ChangelogHistory represents a change event.
type ChangelogHistory struct {
	ID      string          `json:"id"`
	Author  *User           `json:"author"`
	Created string          `json:"created"`
	Items   []ChangelogItem `json:"items"`
}

// ChangelogResponse represents issue history.
type ChangelogResponse struct {
	StartAt    int                `json:"startAt"`
	MaxResults int                `json:"maxResults"`
	Total      int                `json:"total"`
	Histories  []ChangelogHistory `json:"histories"`
}

// CreateIssueRequest is the payload to create an issue.
type CreateIssueRequest struct {
	ProjectKey   string         `json:"projectKey"`
	IssueType    string         `json:"issueType"`
	Summary      string         `json:"summary"`
	Description  string         `json:"description,omitempty"`
	Template     string         `json:"template,omitempty"` // "story", "bug", "spike"
	Priority     string         `json:"priority,omitempty"`
	Assignee     string         `json:"assignee,omitempty"` // accountId, username, or "@me"
	Labels       []string       `json:"labels,omitempty"`
	Components   []string       `json:"components,omitempty"`
	FixVersions  []string       `json:"fixVersions,omitempty"`
	DueDate      string         `json:"dueDate,omitempty"` // YYYY-MM-DD
	ParentKey    string         `json:"parentKey,omitempty"`
	CustomFields map[string]any `json:"customFields,omitempty"`
}

// UnmarshalJSON supports both camelCase and snake_case for LLM compatibility.
func (r *CreateIssueRequest) UnmarshalJSON(data []byte) error {
	type Alias CreateIssueRequest
	aux := &struct {
		ProjectKeySnake   string         `json:"project_key"`
		IssueTypeSnake    string         `json:"issue_type"`
		ParentKeySnake    string         `json:"parent_key"`
		DueDateSnake      string         `json:"due_date"`
		FixVersionsSnake  []string       `json:"fix_versions"`
		CustomFieldsSnake map[string]any `json:"custom_fields"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if r.ProjectKey == "" && aux.ProjectKeySnake != "" {
		r.ProjectKey = aux.ProjectKeySnake
	}
	if r.IssueType == "" && aux.IssueTypeSnake != "" {
		r.IssueType = aux.IssueTypeSnake
	}
	if r.ParentKey == "" && aux.ParentKeySnake != "" {
		r.ParentKey = aux.ParentKeySnake
	}
	if r.DueDate == "" && aux.DueDateSnake != "" {
		r.DueDate = aux.DueDateSnake
	}
	if len(r.FixVersions) == 0 && len(aux.FixVersionsSnake) > 0 {
		r.FixVersions = aux.FixVersionsSnake
	}
	if r.CustomFields == nil && aux.CustomFieldsSnake != nil {
		r.CustomFields = aux.CustomFieldsSnake
	}
	return nil
}

// UpdateIssueRequest is the payload to update issue fields.
type UpdateIssueRequest struct {
	Summary      string         `json:"summary,omitempty"`
	Description  string         `json:"description,omitempty"`
	Priority     string         `json:"priority,omitempty"`
	Assignee     string         `json:"assignee,omitempty"`     // accountId, username, or "@me"
	Labels       []string       `json:"labels,omitempty"`       // Full replacement
	AddLabels    []string       `json:"addLabels,omitempty"`    // Atomic append without overwriting
	RemoveLabels []string       `json:"removeLabels,omitempty"` // Atomic removal
	Components   []string       `json:"components,omitempty"`
	FixVersions  []string       `json:"fixVersions,omitempty"`
	DueDate      string         `json:"dueDate,omitempty"`
	CustomFields map[string]any `json:"customFields,omitempty"`
}

// UnmarshalJSON supports both camelCase and snake_case for LLM compatibility.
func (r *UpdateIssueRequest) UnmarshalJSON(data []byte) error {
	type Alias UpdateIssueRequest
	aux := &struct {
		AddLabelsSnake    []string       `json:"add_labels"`
		RemoveLabelsSnake []string       `json:"remove_labels"`
		FixVersionsSnake  []string       `json:"fix_versions"`
		DueDateSnake      string         `json:"due_date"`
		CustomFieldsSnake map[string]any `json:"custom_fields"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if len(r.AddLabels) == 0 && len(aux.AddLabelsSnake) > 0 {
		r.AddLabels = aux.AddLabelsSnake
	}
	if len(r.RemoveLabels) == 0 && len(aux.RemoveLabelsSnake) > 0 {
		r.RemoveLabels = aux.RemoveLabelsSnake
	}
	if len(r.FixVersions) == 0 && len(aux.FixVersionsSnake) > 0 {
		r.FixVersions = aux.FixVersionsSnake
	}
	if r.DueDate == "" && aux.DueDateSnake != "" {
		r.DueDate = aux.DueDateSnake
	}
	if r.CustomFields == nil && aux.CustomFieldsSnake != nil {
		r.CustomFields = aux.CustomFieldsSnake
	}
	return nil
}

// BulkOperationResult holds the outcome of a bulk action.
type BulkOperationResult struct {
	TotalProcessed int      `json:"totalProcessed"`
	SuccessCount   int      `json:"successCount"`
	FailureCount   int      `json:"failureCount"`
	SuccessfulKeys []string `json:"successfulKeys"`
	Errors         []string `json:"errors,omitempty"`
}

// BacklogAnalysis represents health and statistical breakdown of a backlog.
type BacklogAnalysis struct {
	TotalIssues         int            `json:"totalIssues"`
	EstimatedIssues     int            `json:"estimatedIssues"`
	UnestimatedIssues   int            `json:"unestimatedIssues"`
	UnassignedIssues    int            `json:"unassignedIssues"`
	StaleIssuesCount    int            `json:"staleIssuesCount"` // No updates in > 30 days
	BreakdownByType     map[string]int `json:"breakdownByType"`
	BreakdownByPriority map[string]int `json:"breakdownByPriority"`
	TopLabels           map[string]int `json:"topLabels"`
	SampleStaleKeys     []string       `json:"sampleStaleKeys,omitempty"`
}

// SprintAnalysis represents sprint velocity, progress, and workload breakdown.
type SprintAnalysis struct {
	SprintID            int            `json:"sprintId"`
	SprintName          string         `json:"sprintName"`
	State               string         `json:"state"`
	TotalIssues         int            `json:"totalIssues"`
	CompletedIssues     int            `json:"completedIssues"`
	InProgressIssues    int            `json:"inProgressIssues"`
	ToDoIssues          int            `json:"toDoIssues"`
	CompletionRate      float64        `json:"completionRatePercent"`
	WorkloadByAssignee  map[string]int `json:"workloadByAssignee"`
	BreakdownByType     map[string]int `json:"breakdownByType"`
	BreakdownByPriority map[string]int `json:"breakdownByPriority"`
}
