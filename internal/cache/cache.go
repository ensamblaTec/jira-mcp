package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ensamblatec/jira-mcp/internal/jira"
	_ "modernc.org/sqlite"
)

// CachedIssue represents an issue stored in the local SQLite cache.
type CachedIssue struct {
	Key            string    `json:"key"`
	ID             string    `json:"id"`
	ProjectKey     string    `json:"projectKey"`
	IssueType      string    `json:"issueType"`
	Summary        string    `json:"summary"`
	Description    string    `json:"description"`
	Status         string    `json:"status"`
	StatusCategory string    `json:"statusCategory"`
	Priority       string    `json:"priority"`
	AssigneeID     string    `json:"assigneeId,omitempty"`
	AssigneeName   string    `json:"assigneeName,omitempty"`
	ReporterName   string    `json:"reporterName,omitempty"`
	Labels         []string  `json:"labels"`
	Components     []string  `json:"components"`
	ParentKey      string    `json:"parentKey,omitempty"`
	EpicKey        string    `json:"epicKey,omitempty"`
	CreatedAt      string    `json:"createdAt"`
	UpdatedAt      string    `json:"updatedAt"`
	DueDate        string    `json:"dueDate,omitempty"`
	SyncedAt       time.Time `json:"syncedAt"`
}

// FTSResult represents a contextual search match scored by BM25 with text snippets.
type FTSResult struct {
	Key       string  `json:"key"`
	Summary   string  `json:"summary"`
	Status    string  `json:"status"`
	IssueType string  `json:"issueType"`
	Assignee  string  `json:"assignee"`
	Snippet   string  `json:"snippet"`
	Rank      float64 `json:"rank"`
}

// ContextPack consolidates the complete operational context of a ticket for LLMs.
type ContextPack struct {
	Issue     CachedIssue             `json:"issue"`
	Parent    *CachedIssue            `json:"parent,omitempty"`
	Siblings  []CachedIssue           `json:"siblings,omitempty"`
	Subtasks  []CachedIssue           `json:"subtasks,omitempty"`
	BlockedBy []string                `json:"blockedBy,omitempty"`
	Blocks    []string                `json:"blocks,omitempty"`
	Related   []string                `json:"related,omitempty"`
	Comments  []jira.Comment          `json:"recentComments,omitempty"`
	Changelog []jira.ChangelogHistory `json:"recentHistory,omitempty"`
}

// CacheStats provides metadata about current local cache.
type CacheStats struct {
	DBPath            string            `json:"dbPath"`
	TotalIssues       int               `json:"totalIssues"`
	TotalRelations    int               `json:"totalRelations"`
	ProjectBreakdown  map[string]int    `json:"projectBreakdown"`
	LastSyncByProject map[string]string `json:"lastSyncByProject"`
	DatabaseSizeBytes int64             `json:"databaseSizeBytes"`
	FTSIndexed        bool              `json:"ftsIndexed"`
}

// Cache manages the local SQLite database for Jira issues.
type Cache struct {
	db     *sql.DB
	dbPath string
}

// New initializes the SQLite database and runs migrations.
func New(dbPath string) (*Cache, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache directory '%s': %w", dir, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	// Enforce single-writer serialization at Go driver level to prevent SQLite busy/lock contention
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	c := &Cache{
		db:     db,
		dbPath: dbPath,
	}

	if err := c.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing cache schema: %w", err)
	}

	return c, nil
}

// Close closes the database connection.
func (c *Cache) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

func (c *Cache) initSchema() error {
	_, _ = c.db.Exec("PRAGMA journal_mode=WAL;")
	_, _ = c.db.Exec("PRAGMA busy_timeout=5000;")
	_, _ = c.db.Exec("PRAGMA synchronous=NORMAL;")

	baseSchema := `
	CREATE TABLE IF NOT EXISTS cached_issues (
		key TEXT PRIMARY KEY,
		id TEXT,
		project_key TEXT,
		issue_type TEXT,
		summary TEXT,
		description TEXT,
		status TEXT,
		status_category TEXT,
		priority TEXT,
		assignee_id TEXT,
		assignee_name TEXT,
		reporter_name TEXT,
		labels TEXT,
		components TEXT,
		parent_key TEXT,
		epic_key TEXT,
		created_at TEXT,
		updated_at TEXT,
		due_date TEXT,
		synced_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_issues_proj ON cached_issues(project_key);
	CREATE INDEX IF NOT EXISTS idx_issues_status ON cached_issues(status);
	CREATE INDEX IF NOT EXISTS idx_issues_assignee ON cached_issues(assignee_name);
	CREATE INDEX IF NOT EXISTS idx_issues_updated ON cached_issues(updated_at);

	CREATE TABLE IF NOT EXISTS sync_metadata (
		project_key TEXT PRIMARY KEY,
		last_sync_timestamp TEXT,
		total_items INTEGER DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS issue_relations (
		from_key TEXT,
		to_key TEXT,
		relation_type TEXT,
		relation_desc TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY(from_key, to_key, relation_type)
	);

	CREATE INDEX IF NOT EXISTS idx_rel_from ON issue_relations(from_key);
	CREATE INDEX IF NOT EXISTS idx_rel_to ON issue_relations(to_key);

	CREATE TABLE IF NOT EXISTS cached_boards (
		board_id INTEGER PRIMARY KEY,
		name TEXT,
		board_type TEXT,
		project_key TEXT,
		configuration_json TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_boards_proj ON cached_boards(project_key);
	`
	if _, err := c.db.Exec(baseSchema); err != nil {
		return fmt.Errorf("creating base tables: %w", err)
	}

	// Safe column migrations for pre-existing databases before index creation
	_, _ = c.db.Exec("ALTER TABLE cached_issues ADD COLUMN parent_key TEXT;")
	_, _ = c.db.Exec("ALTER TABLE cached_issues ADD COLUMN epic_key TEXT;")

	// Indexes on parent and epic columns
	_, _ = c.db.Exec("CREATE INDEX IF NOT EXISTS idx_issues_parent ON cached_issues(parent_key);")
	_, _ = c.db.Exec("CREATE INDEX IF NOT EXISTS idx_issues_epic ON cached_issues(epic_key);")

	// Recreate FTS5 table if it was previously created with key UNINDEXED
	var ftsSQL string
	_ = c.db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='issues_fts'").Scan(&ftsSQL)
	if strings.Contains(strings.ToUpper(ftsSQL), "KEY UNINDEXED") {
		_, _ = c.db.Exec(`
			DROP TRIGGER IF EXISTS issues_ai;
			DROP TRIGGER IF EXISTS issues_ad;
			DROP TRIGGER IF EXISTS issues_au;
			DROP TABLE IF EXISTS issues_fts;
		`)
	}

	ftsSchema := `
	CREATE VIRTUAL TABLE IF NOT EXISTS issues_fts USING fts5(
		key,
		summary,
		description,
		labels,
		components,
		tokenize='unicode61'
	);

	CREATE TRIGGER IF NOT EXISTS issues_ai AFTER INSERT ON cached_issues BEGIN
		INSERT INTO issues_fts(key, summary, description, labels, components)
		VALUES (new.key, new.summary, coalesce(new.description, ''), coalesce(new.labels, ''), coalesce(new.components, ''));
	END;

	CREATE TRIGGER IF NOT EXISTS issues_ad AFTER DELETE ON cached_issues BEGIN
		DELETE FROM issues_fts WHERE key = old.key;
	END;

	CREATE TRIGGER IF NOT EXISTS issues_au AFTER UPDATE ON cached_issues BEGIN
		DELETE FROM issues_fts WHERE key = old.key;
		INSERT INTO issues_fts(key, summary, description, labels, components)
		VALUES (new.key, new.summary, coalesce(new.description, ''), coalesce(new.labels, ''), coalesce(new.components, ''));
	END;
	`
	if _, err := c.db.Exec(ftsSchema); err != nil {
		return fmt.Errorf("creating FTS5 schema: %w", err)
	}

	// Backfill issues_fts if there are issues in cached_issues not yet indexed
	_, _ = c.db.Exec(`
	INSERT INTO issues_fts (key, summary, description, labels, components)
	SELECT key, summary, coalesce(description, ''), coalesce(labels, ''), coalesce(components, '')
	FROM cached_issues
	WHERE key NOT IN (SELECT key FROM issues_fts);
	`)

	return nil
}

// UpsertIssue inserts or updates an issue in the local cache.
func (c *Cache) UpsertIssue(issue *jira.Issue) error {
	return c.UpsertIssues([]jira.Issue{*issue})
}

// UpsertIssues performs a batch transaction insert/update into local SQLite.
func (c *Cache) UpsertIssues(issues []jira.Issue) error {
	if len(issues) == 0 {
		return nil
	}

	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
	INSERT INTO cached_issues (
		key, id, project_key, issue_type, summary, description,
		status, status_category, priority, assignee_id, assignee_name,
		reporter_name, labels, components, parent_key, epic_key,
		created_at, updated_at, due_date, synced_at
	) VALUES (
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?, CURRENT_TIMESTAMP
	)
	ON CONFLICT(key) DO UPDATE SET
		id = excluded.id,
		project_key = excluded.project_key,
		issue_type = excluded.issue_type,
		summary = excluded.summary,
		description = excluded.description,
		status = excluded.status,
		status_category = excluded.status_category,
		priority = excluded.priority,
		assignee_id = excluded.assignee_id,
		assignee_name = excluded.assignee_name,
		reporter_name = excluded.reporter_name,
		labels = excluded.labels,
		components = excluded.components,
		parent_key = CASE WHEN excluded.parent_key != '' THEN excluded.parent_key ELSE cached_issues.parent_key END,
		epic_key = CASE WHEN excluded.epic_key != '' THEN excluded.epic_key ELSE cached_issues.epic_key END,
		created_at = excluded.created_at,
		updated_at = excluded.updated_at,
		due_date = excluded.due_date,
		synced_at = CURRENT_TIMESTAMP;
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	relStmt, err := tx.Prepare(`
	INSERT INTO issue_relations (from_key, to_key, relation_type, relation_desc, created_at)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(from_key, to_key, relation_type) DO UPDATE SET
		relation_desc = excluded.relation_desc,
		created_at = CURRENT_TIMESTAMP;
	`)
	if err != nil {
		return fmt.Errorf("preparing relations statement: %w", err)
	}
	defer relStmt.Close()

	subParentStmt, err := tx.Prepare(`UPDATE cached_issues SET parent_key = ? WHERE key = ? AND (parent_key IS NULL OR parent_key = '')`)
	if err != nil {
		return fmt.Errorf("preparing subtask parent statement: %w", err)
	}
	defer subParentStmt.Close()

	for _, issue := range issues {
		projKey := ""
		if issue.Fields.Project != nil {
			projKey = issue.Fields.Project.Key
		} else if strings.Contains(issue.Key, "-") {
			parts := strings.Split(issue.Key, "-")
			projKey = parts[0]
		}

		issueType := ""
		if issue.Fields.IssueType != nil {
			issueType = issue.Fields.IssueType.Name
		}

		statusName := ""
		statusCat := ""
		if issue.Fields.Status != nil {
			statusName = issue.Fields.Status.Name
			if issue.Fields.Status.StatusCategory != nil {
				statusCat = issue.Fields.Status.StatusCategory.Name
			}
		}

		priorityName := ""
		if issue.Fields.Priority != nil {
			priorityName = issue.Fields.Priority.Name
		}

		assigneeID := ""
		assigneeName := ""
		if issue.Fields.Assignee != nil {
			assigneeID = issue.Fields.Assignee.AccountID
			assigneeName = issue.Fields.Assignee.DisplayName
		}

		reporterName := ""
		if issue.Fields.Reporter != nil {
			reporterName = issue.Fields.Reporter.DisplayName
		}

		descStr := ""
		if issue.Fields.Description != nil {
			if s, ok := issue.Fields.Description.(string); ok {
				descStr = s
			} else {
				data, _ := json.Marshal(issue.Fields.Description)
				descStr = string(data)
			}
		}

		labelsJSON, _ := json.Marshal(issue.Fields.Labels)

		compNames := make([]string, 0, len(issue.Fields.Components))
		for _, comp := range issue.Fields.Components {
			compNames = append(compNames, comp.Name)
		}
		compJSON, _ := json.Marshal(compNames)

		// Extract Parent and Epic keys
		parentKey := ""
		epicKey := ""
		if issue.Fields.Parent != nil {
			parentKey = issue.Fields.Parent.Key
			if issue.Fields.Parent.Fields.IssueType != nil && strings.ToLower(issue.Fields.Parent.Fields.IssueType.Name) == "epic" {
				epicKey = issue.Fields.Parent.Key
			}
		}
		if epicKey == "" && issue.Fields.CustomFields != nil {
			for k, v := range issue.Fields.CustomFields {
				if strings.Contains(strings.ToLower(k), "epic") {
					if s, ok := v.(string); ok && strings.Contains(s, "-") {
						epicKey = s
						break
					}
				}
			}
		}

		_, err := stmt.Exec(
			issue.Key,
			issue.ID,
			projKey,
			issueType,
			issue.Fields.Summary,
			descStr,
			statusName,
			statusCat,
			priorityName,
			assigneeID,
			assigneeName,
			reporterName,
			string(labelsJSON),
			string(compJSON),
			parentKey,
			epicKey,
			issue.Fields.Created,
			issue.Fields.Updated,
			issue.Fields.DueDate,
		)
		if err != nil {
			return fmt.Errorf("executing upsert for %s: %w", issue.Key, err)
		}

		// Record Issue Links
		for _, link := range issue.Fields.IssueLinks {
			rType := "Relates"
			inDesc := "is related to"
			outDesc := "is related to"
			if link.Type != nil {
				rType = link.Type.Name
				inDesc = link.Type.Inward
				outDesc = link.Type.Outward
			}
			if link.InwardIssue != nil {
				_, _ = relStmt.Exec(issue.Key, link.InwardIssue.Key, rType, inDesc)
			}
			if link.OutwardIssue != nil {
				_, _ = relStmt.Exec(issue.Key, link.OutwardIssue.Key, rType, outDesc)
			}
		}

		// Link subtasks to parent
		for _, sub := range issue.Fields.Subtasks {
			if sub.Key != "" {
				_, _ = subParentStmt.Exec(issue.Key, sub.Key)
			}
		}
	}

	return tx.Commit()
}

// GetIssue retrieves an issue from the local cache.
func (c *Cache) GetIssue(key string) (*CachedIssue, error) {
	query := `
	SELECT key, id, project_key, issue_type, summary, description,
	       status, status_category, priority, assignee_id, assignee_name,
	       reporter_name, labels, components, parent_key, epic_key,
	       created_at, updated_at, due_date, synced_at
	FROM cached_issues WHERE key = ?
	`
	row := c.db.QueryRow(query, key)

	var item CachedIssue
	var labelsRaw, compsRaw string
	var parentKey, epicKey sql.NullString
	err := row.Scan(
		&item.Key, &item.ID, &item.ProjectKey, &item.IssueType, &item.Summary, &item.Description,
		&item.Status, &item.StatusCategory, &item.Priority, &item.AssigneeID, &item.AssigneeName,
		&item.ReporterName, &labelsRaw, &compsRaw, &parentKey, &epicKey,
		&item.CreatedAt, &item.UpdatedAt, &item.DueDate, &item.SyncedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning cached issue %s: %w", key, err)
	}

	item.ParentKey = parentKey.String
	item.EpicKey = epicKey.String
	_ = json.Unmarshal([]byte(labelsRaw), &item.Labels)
	_ = json.Unmarshal([]byte(compsRaw), &item.Components)

	return &item, nil
}

// DeleteIssue removes an issue and its orphaned relations from the local cache.
func (c *Cache) DeleteIssue(key string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning delete transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM issue_relations WHERE from_key = ? OR to_key = ?", key, key); err != nil {
		return fmt.Errorf("deleting issue relations: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM cached_issues WHERE key = ?", key); err != nil {
		return fmt.Errorf("deleting cached issue: %w", err)
	}
	return tx.Commit()
}

// Search searches cached issues using SQL filters with fast indexing.
func (c *Cache) Search(query string, projectKey string, status string, assignee string, priority string, limit int) ([]CachedIssue, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var conditions []string
	var args []any

	if query != "" {
		conditions = append(conditions, "(key LIKE ? OR summary LIKE ? OR description LIKE ?)")
		searchTerm := "%" + query + "%"
		args = append(args, searchTerm, searchTerm, searchTerm)
	}

	if projectKey != "" {
		conditions = append(conditions, "project_key = ?")
		args = append(args, projectKey)
	}

	if status != "" {
		conditions = append(conditions, "status LIKE ?")
		args = append(args, "%"+status+"%")
	}

	if assignee != "" {
		conditions = append(conditions, "assignee_name LIKE ?")
		args = append(args, "%"+assignee+"%")
	}

	if priority != "" {
		conditions = append(conditions, "priority LIKE ?")
		args = append(args, "%"+priority+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	sqlQuery := fmt.Sprintf(`
	SELECT key, id, project_key, issue_type, summary, description,
	       status, status_category, priority, assignee_id, assignee_name,
	       reporter_name, labels, components, parent_key, epic_key,
	       created_at, updated_at, due_date, synced_at
	FROM cached_issues
	%s
	ORDER BY updated_at DESC
	LIMIT ?
	`, whereClause)
	args = append(args, limit)

	rows, err := c.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("querying cached issues: %w", err)
	}
	defer rows.Close()

	var results []CachedIssue
	for rows.Next() {
		var item CachedIssue
		var labelsRaw, compsRaw string
		var parentKey, epicKey sql.NullString
		if err := rows.Scan(
			&item.Key, &item.ID, &item.ProjectKey, &item.IssueType, &item.Summary, &item.Description,
			&item.Status, &item.StatusCategory, &item.Priority, &item.AssigneeID, &item.AssigneeName,
			&item.ReporterName, &labelsRaw, &compsRaw, &parentKey, &epicKey,
			&item.CreatedAt, &item.UpdatedAt, &item.DueDate, &item.SyncedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		item.ParentKey = parentKey.String
		item.EpicKey = epicKey.String
		_ = json.Unmarshal([]byte(labelsRaw), &item.Labels)
		_ = json.Unmarshal([]byte(compsRaw), &item.Components)
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating cached issues rows: %w", err)
	}

	return results, nil
}

// GetLastSyncTime returns the latest updated_at timestamp stored for a project.
func (c *Cache) GetLastSyncTime(projectKey string) (string, error) {
	var ts sql.NullString
	err := c.db.QueryRow("SELECT last_sync_timestamp FROM sync_metadata WHERE project_key = ?", projectKey).Scan(&ts)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return ts.String, nil
}

// SetLastSyncTime records the latest sync timestamp for a project.
func (c *Cache) SetLastSyncTime(projectKey string, timestamp string, total int) error {
	query := `
	INSERT INTO sync_metadata (project_key, last_sync_timestamp, total_items, updated_at)
	VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(project_key) DO UPDATE SET
		last_sync_timestamp = excluded.last_sync_timestamp,
		total_items = excluded.total_items,
		updated_at = CURRENT_TIMESTAMP;
	`
	_, err := c.db.Exec(query, projectKey, timestamp, total)
	return err
}

// GetStats returns summary statistics of the cache.
func (c *Cache) GetStats() (*CacheStats, error) {
	stats := &CacheStats{
		DBPath:            c.dbPath,
		ProjectBreakdown:  make(map[string]int),
		LastSyncByProject: make(map[string]string),
	}

	// Total issues
	var total int
	if err := c.db.QueryRow("SELECT count(*) FROM cached_issues").Scan(&total); err == nil {
		stats.TotalIssues = total
	}

	// Project breakdown
	rows, err := c.db.Query("SELECT project_key, count(*) FROM cached_issues GROUP BY project_key")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k string
			var count int
			if err := rows.Scan(&k, &count); err == nil {
				stats.ProjectBreakdown[k] = count
			}
		}
	}

	// Sync metadata
	metaRows, err := c.db.Query("SELECT project_key, last_sync_timestamp FROM sync_metadata")
	if err == nil {
		defer metaRows.Close()
		for metaRows.Next() {
			var k, ts string
			if err := metaRows.Scan(&k, &ts); err == nil {
				stats.LastSyncByProject[k] = ts
			}
		}
	}

	// Total relations
	var totalRel int
	if err := c.db.QueryRow("SELECT count(*) FROM issue_relations").Scan(&totalRel); err == nil {
		stats.TotalRelations = totalRel
	}

	// FTS indexed status
	var ftsCount int
	if err := c.db.QueryRow("SELECT count(*) FROM issues_fts").Scan(&ftsCount); err == nil {
		stats.FTSIndexed = ftsCount > 0
	}

	// File size
	if fi, err := os.Stat(c.dbPath); err == nil {
		stats.DatabaseSizeBytes = fi.Size()
	}

	return stats, nil
}

// SyncIncremental fetches only new/updated issues from Jira and saves them to local cache.
func (c *Cache) SyncIncremental(ctx context.Context, client *jira.Client, projectKey string, fullSync bool) (int, string, error) {
	lastSync := ""
	if !fullSync {
		lastSync, _ = c.GetLastSyncTime(projectKey)
	}

	var jqlParts []string
	if projectKey != "" {
		jqlParts = append(jqlParts, fmt.Sprintf("project = '%s'", projectKey))
	}

	// If we have a previous sync timestamp, fetch only updated >= 'YYYY-MM-DD HH:mm'
	if lastSync != "" && !fullSync {
		// Jira JQL format for date filter is 'YYYY-MM-DD HH:mm' or 'YYYY-MM-DD'
		jqlDate := lastSync
		if len(jqlDate) >= 16 {
			jqlDate = jqlDate[:16] // Keep YYYY-MM-DD HH:mm
			jqlDate = strings.Replace(jqlDate, "T", " ", 1)
		}
		jqlParts = append(jqlParts, fmt.Sprintf("updated >= '%s'", jqlDate))
	}

	if len(jqlParts) == 0 {
		jqlParts = append(jqlParts, "id is not null")
	}

	jqlQuery := strings.Join(jqlParts, " AND ") + " ORDER BY updated ASC"

	startAt := 0
	pageSize := 50
	totalSynced := 0
	latestUpdateTimestamp := lastSync
	pageToken := ""

	for {
		var result *jira.SearchResult
		var err error
		if pageToken != "" {
			result, err = client.SearchIssues(ctx, jqlQuery, nil, 0, pageSize, pageToken)
		} else {
			result, err = client.SearchIssues(ctx, jqlQuery, nil, startAt, pageSize)
		}
		if err != nil {
			return totalSynced, latestUpdateTimestamp, fmt.Errorf("fetching issues with JQL '%s': %w", jqlQuery, err)
		}

		if len(result.Issues) == 0 {
			break
		}

		if err := c.UpsertIssues(result.Issues); err != nil {
			return totalSynced, latestUpdateTimestamp, fmt.Errorf("caching issues: %w", err)
		}

		totalSynced += len(result.Issues)

		for _, issue := range result.Issues {
			if issue.Fields.Updated > latestUpdateTimestamp {
				latestUpdateTimestamp = issue.Fields.Updated
			}
		}

		if result.IsLast {
			break
		}

		if result.NextPageToken != "" {
			pageToken = result.NextPageToken
		} else {
			startAt += len(result.Issues)
			if (result.Total > 0 && startAt >= result.Total) || len(result.Issues) < pageSize {
				break
			}
		}
	}

	if projectKey != "" && latestUpdateTimestamp != "" {
		_ = c.SetLastSyncTime(projectKey, latestUpdateTimestamp, totalSynced)
	}

	return totalSynced, latestUpdateTimestamp, nil
}

// CleanFTSQuery prepares user query terms for safe FTS5 MATCH syntax with prefix matching.
func CleanFTSQuery(query string) string {
	// Strip internal and boundary quotes to avoid broken syntax in FTS5 MATCH expressions (e.g. 24" -> "24""*)
	query = strings.ReplaceAll(query, "\"", " ")
	words := strings.Fields(query)
	if len(words) == 0 {
		return ""
	}
	var cleaned []string
	for _, w := range words {
		w = strings.Trim(w, `"'()*:^~`)
		if w == "" {
			continue
		}
		if strings.ContainsAny(w, "-_/") {
			cleaned = append(cleaned, `"`+w+`"`)
		} else {
			cleaned = append(cleaned, `"`+w+`"*`)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	return strings.Join(cleaned, " ")
}

// SearchContext searches issues in the SQLite FTS5 index ranked by BM25 relevance with text snippets.
func (c *Cache) SearchContext(query string, projectKey, status, assignee, priority string, limit int) ([]FTSResult, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}

	ftsQuery := CleanFTSQuery(query)
	if ftsQuery == "" {
		cached, err := c.Search("", projectKey, status, assignee, priority, limit)
		if err != nil {
			return nil, err
		}
		results := make([]FTSResult, 0, len(cached))
		for _, issue := range cached {
			snipRunes := []rune(issue.Description)
			snip := string(snipRunes)
			if len(snipRunes) > 120 {
				snip = string(snipRunes[:120]) + "..."
			}
			results = append(results, FTSResult{
				Key:       issue.Key,
				Summary:   issue.Summary,
				Status:    issue.Status,
				IssueType: issue.IssueType,
				Assignee:  issue.AssigneeName,
				Snippet:   snip,
				Rank:      0.0,
			})
		}
		return results, nil
	}

	var conditions []string
	var args []any

	conditions = append(conditions, "issues_fts MATCH ?")
	args = append(args, ftsQuery)

	if projectKey != "" {
		conditions = append(conditions, "ci.project_key = ?")
		args = append(args, projectKey)
	}
	if status != "" {
		conditions = append(conditions, "ci.status LIKE ?")
		args = append(args, "%"+status+"%")
	}
	if assignee != "" {
		conditions = append(conditions, "ci.assignee_name LIKE ?")
		args = append(args, "%"+assignee+"%")
	}
	if priority != "" {
		conditions = append(conditions, "ci.priority LIKE ?")
		args = append(args, "%"+priority+"%")
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	sqlQuery := fmt.Sprintf(`
	SELECT
		ci.key,
		ci.summary,
		ci.status,
		ci.issue_type,
		ci.assignee_name,
		snippet(issues_fts, -1, '==', '==', '...', 15) as snip,
		bm25(issues_fts) as rank
	FROM issues_fts
	JOIN cached_issues ci ON ci.key = issues_fts.key
	%s
	ORDER BY rank ASC
	LIMIT ?
	`, whereClause)
	args = append(args, limit)

	rows, err := c.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("FTS context search error: %w", err)
	}
	defer rows.Close()

	var results []FTSResult
	for rows.Next() {
		var r FTSResult
		if err := rows.Scan(&r.Key, &r.Summary, &r.Status, &r.IssueType, &r.Assignee, &r.Snippet, &r.Rank); err != nil {
			return nil, fmt.Errorf("scanning FTS result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating FTS results: %w", err)
	}

	return results, nil
}

// GetContextPack builds a deep operational context graph for a ticket (combining Parent/Epic, Siblings, Subtasks, Dependencies, Comments, History).
func (c *Cache) GetContextPack(ctx context.Context, client *jira.Client, issueKey string) (*ContextPack, error) {
	issueKey = strings.TrimSpace(strings.ToUpper(issueKey))
	if issueKey == "" {
		return nil, fmt.Errorf("issue_key cannot be empty")
	}

	// 1. Get main issue from cache or Jira
	cached, err := c.GetIssue(issueKey)
	if err != nil {
		return nil, fmt.Errorf("fetching cached issue '%s': %w", issueKey, err)
	}

	var liveIssue *jira.Issue
	if cached == nil && client != nil {
		live, err := client.GetIssue(ctx, issueKey, nil)
		if err != nil {
			return nil, fmt.Errorf("fetching issue '%s' from Jira: %w", issueKey, err)
		}
		_ = c.UpsertIssue(live)
		cached, _ = c.GetIssue(issueKey)
		liveIssue = live
	}

	if cached == nil {
		return nil, fmt.Errorf("issue '%s' not found in local cache or Jira", issueKey)
	}

	pack := &ContextPack{
		Issue:     *cached,
		Siblings:  []CachedIssue{},
		Subtasks:  []CachedIssue{},
		BlockedBy: []string{},
		Blocks:    []string{},
		Related:   []string{},
		Comments:  []jira.Comment{},
		Changelog: []jira.ChangelogHistory{},
	}

	// 2. Resolve Parent / Epic
	parentKey := cached.ParentKey
	if parentKey == "" && cached.EpicKey != "" {
		parentKey = cached.EpicKey
	}
	if parentKey != "" {
		parent, err := c.GetIssue(parentKey)
		if err == nil && parent != nil {
			pack.Parent = parent
		} else if client != nil {
			if liveParent, err := client.GetIssue(ctx, parentKey, nil); err == nil && liveParent != nil {
				_ = c.UpsertIssue(liveParent)
				if p, _ := c.GetIssue(parentKey); p != nil {
					pack.Parent = p
				}
			}
		}

		// 3. Resolve Siblings (other tickets under the same parent)
		sibRows, err := c.db.Query(`
			SELECT key, id, project_key, issue_type, summary, description,
			       status, status_category, priority, assignee_id, assignee_name,
			       reporter_name, labels, components, parent_key, epic_key,
			       created_at, updated_at, due_date, synced_at
			FROM cached_issues
			WHERE (parent_key = ? OR epic_key = ?) AND key != ?
			ORDER BY key ASC LIMIT 20
		`, parentKey, parentKey, issueKey)
		if err == nil {
			defer sibRows.Close()
			for sibRows.Next() {
				var sib CachedIssue
				var labelsRaw, compsRaw string
				var pKey, eKey sql.NullString
				if err := sibRows.Scan(
					&sib.Key, &sib.ID, &sib.ProjectKey, &sib.IssueType, &sib.Summary, &sib.Description,
					&sib.Status, &sib.StatusCategory, &sib.Priority, &sib.AssigneeID, &sib.AssigneeName,
					&sib.ReporterName, &labelsRaw, &compsRaw, &pKey, &eKey,
					&sib.CreatedAt, &sib.UpdatedAt, &sib.DueDate, &sib.SyncedAt,
				); err == nil {
					sib.ParentKey = pKey.String
					sib.EpicKey = eKey.String
					_ = json.Unmarshal([]byte(labelsRaw), &sib.Labels)
					_ = json.Unmarshal([]byte(compsRaw), &sib.Components)
					pack.Siblings = append(pack.Siblings, sib)
				}
			}
		}
	}

	// 4. Resolve Subtasks
	subRows, err := c.db.Query(`
		SELECT key, id, project_key, issue_type, summary, description,
		       status, status_category, priority, assignee_id, assignee_name,
		       reporter_name, labels, components, parent_key, epic_key,
		       created_at, updated_at, due_date, synced_at
		FROM cached_issues
		WHERE parent_key = ?
		ORDER BY key ASC LIMIT 50
	`, issueKey)
	if err == nil {
		defer subRows.Close()
		for subRows.Next() {
			var sub CachedIssue
			var labelsRaw, compsRaw string
			var pKey, eKey sql.NullString
			if err := subRows.Scan(
				&sub.Key, &sub.ID, &sub.ProjectKey, &sub.IssueType, &sub.Summary, &sub.Description,
				&sub.Status, &sub.StatusCategory, &sub.Priority, &sub.AssigneeID, &sub.AssigneeName,
				&sub.ReporterName, &labelsRaw, &compsRaw, &pKey, &eKey,
				&sub.CreatedAt, &sub.UpdatedAt, &sub.DueDate, &sub.SyncedAt,
			); err == nil {
				sub.ParentKey = pKey.String
				sub.EpicKey = eKey.String
				_ = json.Unmarshal([]byte(labelsRaw), &sub.Labels)
				_ = json.Unmarshal([]byte(compsRaw), &sub.Components)
				pack.Subtasks = append(pack.Subtasks, sub)
			}
		}
	}

	// 5. Resolve Relations (Dependencies) from issue_relations
	// Outgoing relations from this issue (e.g. this issue is blocked by X, or this issue blocks Y)
	outRows, err := c.db.Query(`SELECT to_key, relation_type, relation_desc FROM issue_relations WHERE from_key = ?`, issueKey)
	if err == nil {
		defer outRows.Close()
		for outRows.Next() {
			var toKey, rType, rDesc string
			if err := outRows.Scan(&toKey, &rType, &rDesc); err == nil {
				lower := strings.ToLower(rDesc)
				if strings.Contains(lower, "blocked by") {
					pack.BlockedBy = append(pack.BlockedBy, toKey)
				} else if strings.Contains(lower, "blocks") {
					pack.Blocks = append(pack.Blocks, toKey)
				} else {
					pack.Related = append(pack.Related, fmt.Sprintf("%s (%s)", toKey, rType))
				}
			}
		}
	}

	// Incoming relations to this issue (e.g. X is blocked by this issue, or X blocks this issue)
	inRows, err := c.db.Query(`SELECT from_key, relation_type, relation_desc FROM issue_relations WHERE to_key = ?`, issueKey)
	if err == nil {
		defer inRows.Close()
		for inRows.Next() {
			var fromKey, rType, rDesc string
			if err := inRows.Scan(&fromKey, &rType, &rDesc); err == nil {
				lower := strings.ToLower(rDesc)
				if strings.Contains(lower, "blocked by") {
					// fromKey is blocked by this issue => this issue blocks fromKey
					pack.Blocks = append(pack.Blocks, fromKey)
				} else if strings.Contains(lower, "blocks") {
					// fromKey blocks this issue => this issue is blocked by fromKey
					pack.BlockedBy = append(pack.BlockedBy, fromKey)
				} else {
					pack.Related = append(pack.Related, fmt.Sprintf("%s (%s)", fromKey, rType))
				}
			}
		}
	}

	// If liveIssue was fetched with issue links, populate from liveIssue if relations table was empty
	if liveIssue != nil && len(pack.BlockedBy) == 0 && len(pack.Blocks) == 0 {
		for _, link := range liveIssue.Fields.IssueLinks {
			relName := "Related"
			if link.Type != nil {
				relName = link.Type.Name
			}
			if link.InwardIssue != nil {
				desc := ""
				if link.Type != nil {
					desc = link.Type.Inward
				}
				if strings.Contains(strings.ToLower(desc), "blocked by") {
					pack.BlockedBy = append(pack.BlockedBy, link.InwardIssue.Key)
				} else {
					pack.Related = append(pack.Related, fmt.Sprintf("%s (%s)", link.InwardIssue.Key, relName))
				}
			}
			if link.OutwardIssue != nil {
				desc := ""
				if link.Type != nil {
					desc = link.Type.Outward
				}
				if strings.Contains(strings.ToLower(desc), "blocks") {
					pack.Blocks = append(pack.Blocks, link.OutwardIssue.Key)
				} else {
					pack.Related = append(pack.Related, fmt.Sprintf("%s (%s)", link.OutwardIssue.Key, relName))
				}
			}
		}
	}

	// 6. Fetch recent comments and changelog if client is provided
	if client != nil {
		if comments, err := client.GetComments(ctx, issueKey); err == nil {
			if len(comments) > 5 {
				pack.Comments = comments[len(comments)-5:]
			} else {
				pack.Comments = comments
			}
		}
		if changelog, err := client.GetChangelog(ctx, issueKey); err == nil && changelog != nil {
			if len(changelog.Histories) > 5 {
				pack.Changelog = changelog.Histories[len(changelog.Histories)-5:]
			} else {
				pack.Changelog = changelog.Histories
			}
		}
	}

	pack.BlockedBy = dedupeStrings(pack.BlockedBy)
	pack.Blocks = dedupeStrings(pack.Blocks)
	pack.Related = dedupeStrings(pack.Related)

	return pack, nil
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, it := range items {
		if _, exists := seen[it]; !exists {
			seen[it] = struct{}{}
			result = append(result, it)
		}
	}
	return result
}

// ToMarkdown formats the ContextPack into a clean, dense Markdown card for LLM comprehension.
func (p *ContextPack) ToMarkdown() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# [%s] %s\n\n", p.Issue.Key, p.Issue.Summary))
	sb.WriteString(fmt.Sprintf("- **Type:** %s | **Status:** %s (%s) | **Priority:** %s\n",
		p.Issue.IssueType, p.Issue.Status, p.Issue.StatusCategory, p.Issue.Priority))
	sb.WriteString(fmt.Sprintf("- **Assignee:** %s | **Reporter:** %s\n",
		nonEmpty(p.Issue.AssigneeName, "Unassigned"), nonEmpty(p.Issue.ReporterName, "None")))
	if len(p.Issue.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("- **Labels:** `%s`\n", strings.Join(p.Issue.Labels, "`, `")))
	}
	if len(p.Issue.Components) > 0 {
		sb.WriteString(fmt.Sprintf("- **Components:** %s\n", strings.Join(p.Issue.Components, ", ")))
	}
	sb.WriteString(fmt.Sprintf("- **Updated:** %s\n\n", p.Issue.UpdatedAt))

	// Description
	sb.WriteString("## Description\n")
	if strings.TrimSpace(p.Issue.Description) != "" {
		sb.WriteString(p.Issue.Description + "\n\n")
	} else {
		sb.WriteString("*(No description provided)*\n\n")
	}

	// Parent / Epic Context
	if p.Parent != nil {
		sb.WriteString("## Hierarchy Context\n")
		sb.WriteString(fmt.Sprintf("- **Parent/Epic:** [%s] %s (Status: %s, Priority: %s)\n",
			p.Parent.Key, p.Parent.Summary, p.Parent.Status, p.Parent.Priority))
		if len(p.Siblings) > 0 {
			sb.WriteString(fmt.Sprintf("- **Sibling Tasks (%d in same parent):**\n", len(p.Siblings)))
			for _, s := range p.Siblings {
				sb.WriteString(fmt.Sprintf("  - [%s] %s (`%s` - %s)\n", s.Key, s.Summary, s.Status, nonEmpty(s.AssigneeName, "Unassigned")))
			}
		}
		sb.WriteString("\n")
	}

	// Subtasks
	if len(p.Subtasks) > 0 {
		sb.WriteString(fmt.Sprintf("## Subtasks (%d)\n", len(p.Subtasks)))
		for _, sub := range p.Subtasks {
			check := "[ ]"
			if strings.EqualFold(sub.StatusCategory, "Done") || strings.EqualFold(sub.Status, "Done") {
				check = "[x]"
			}
			sb.WriteString(fmt.Sprintf("- %s [%s] %s (`%s`)\n", check, sub.Key, sub.Summary, sub.Status))
		}
		sb.WriteString("\n")
	}

	// Dependencies
	if len(p.BlockedBy) > 0 || len(p.Blocks) > 0 || len(p.Related) > 0 {
		sb.WriteString("## Dependencies & Relations\n")
		if len(p.BlockedBy) > 0 {
			sb.WriteString(fmt.Sprintf("- ⛔ **BLOCKED BY:** %s\n", strings.Join(p.BlockedBy, ", ")))
		}
		if len(p.Blocks) > 0 {
			sb.WriteString(fmt.Sprintf("- ⚠️ **BLOCKS:** %s\n", strings.Join(p.Blocks, ", ")))
		}
		if len(p.Related) > 0 {
			sb.WriteString(fmt.Sprintf("- 🔗 **Related Issues:** %s\n", strings.Join(p.Related, ", ")))
		}
		sb.WriteString("\n")
	}

	// Recent Comments
	if len(p.Comments) > 0 {
		sb.WriteString(fmt.Sprintf("## Recent Comments (%d)\n", len(p.Comments)))
		for _, c := range p.Comments {
			author := "User"
			if c.Author != nil && c.Author.DisplayName != "" {
				author = c.Author.DisplayName
			}
			bodyStr := fmt.Sprintf("%v", c.Body)
			if len(bodyStr) > 300 {
				bodyStr = bodyStr[:300] + "..."
			}
			sb.WriteString(fmt.Sprintf("> **%s** (%s):\n> %s\n\n", author, c.Created, bodyStr))
		}
	}

	// Recent Activity
	if len(p.Changelog) > 0 {
		sb.WriteString("## Recent Activity\n")
		for _, h := range p.Changelog {
			for _, item := range h.Items {
				sb.WriteString(fmt.Sprintf("- Changed **%s** from `%s` to `%s`\n", item.Field, item.FromString, item.ToString))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func nonEmpty(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}

// UpsertBoard stores or updates a board and its configuration in SQLite.
func (c *Cache) UpsertBoard(b *jira.Board, config *jira.BoardConfiguration) error {
	if b == nil && config == nil {
		return fmt.Errorf("upserting board: no board or configuration provided")
	}

	boardID := 0
	name := ""
	boardType := "kanban"
	projectKey := ""

	if b != nil {
		boardID = b.ID
		name = b.Name
		boardType = b.Type
	}
	if config != nil {
		if boardID == 0 {
			boardID = config.ID
		}
		if name == "" {
			name = config.Name
		}
		if config.Type != "" {
			boardType = config.Type
		}
		if config.Location != nil && config.Location.ProjectKey != "" {
			projectKey = config.Location.ProjectKey
		}
	}

	// Anti-clobbering: if new config has no columns but an existing cached board has columns, preserve them
	if config != nil && len(config.ColumnConfig.Columns) == 0 && boardID > 0 {
		if existing, _ := c.GetBoard(boardID); existing != nil && len(existing.ColumnConfig.Columns) > 0 {
			config.ColumnConfig = existing.ColumnConfig
		}
	}

	configJSON := ""
	if config != nil {
		if data, err := json.Marshal(config); err == nil {
			configJSON = string(data)
		}
	}

	query := `
	INSERT INTO cached_boards (board_id, name, board_type, project_key, configuration_json, updated_at)
	VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(board_id) DO UPDATE SET
		name = excluded.name,
		board_type = excluded.board_type,
		project_key = CASE WHEN excluded.project_key != '' THEN excluded.project_key ELSE cached_boards.project_key END,
		configuration_json = CASE WHEN excluded.configuration_json != '' THEN excluded.configuration_json ELSE cached_boards.configuration_json END,
		updated_at = CURRENT_TIMESTAMP;
	`
	_, err := c.db.Exec(query, boardID, name, boardType, projectKey, configJSON)
	return err
}

// GetBoard retrieves a cached board's configuration.
func (c *Cache) GetBoard(boardID int) (*jira.BoardConfiguration, error) {
	var configJSON string
	err := c.db.QueryRow("SELECT configuration_json FROM cached_boards WHERE board_id = ?", boardID).Scan(&configJSON)
	if err == sql.ErrNoRows || configJSON == "" {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading cached board %d: %w", boardID, err)
	}

	var config jira.BoardConfiguration
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("unmarshaling cached board %d configuration: %w", boardID, err)
	}
	return &config, nil
}

// ListCachedBoards returns all boards currently indexed in the local cache.
func (c *Cache) ListCachedBoards() ([]jira.Board, error) {
	rows, err := c.db.Query("SELECT board_id, name, board_type FROM cached_boards ORDER BY name ASC")
	if err != nil {
		return nil, fmt.Errorf("listing cached boards: %w", err)
	}
	defer rows.Close()

	var boards []jira.Board
	for rows.Next() {
		var b jira.Board
		if err := rows.Scan(&b.ID, &b.Name, &b.Type); err == nil {
			boards = append(boards, b)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating cached boards: %w", err)
	}
	return boards, nil
}

// GetLocalBoardView constructs a BoardView solely from cached issues in SQLite without touching the network.
func (c *Cache) GetLocalBoardView(boardID int, fallbackProjectKey string, config *jira.BoardConfiguration, includeBacklog bool) (*jira.BoardView, error) {
	boardName := fmt.Sprintf("Board %d", boardID)
	boardType := "kanban"
	projectKey := fallbackProjectKey

	if boardID > 0 {
		var dbName, dbType, dbProjKey, configJSON string
		if c.db.QueryRow("SELECT name, board_type, project_key, configuration_json FROM cached_boards WHERE board_id = ?", boardID).Scan(&dbName, &dbType, &dbProjKey, &configJSON) == nil {
			if dbName != "" {
				boardName = dbName
			}
			if dbType != "" {
				boardType = dbType
			}
			if dbProjKey != "" && projectKey == "" {
				projectKey = dbProjKey
			}
			if config == nil && configJSON != "" {
				var cfg jira.BoardConfiguration
				if json.Unmarshal([]byte(configJSON), &cfg) == nil {
					config = &cfg
				}
			}
		}
	}

	if config != nil {
		if config.Name != "" {
			boardName = config.Name
		}
		if config.Type != "" {
			boardType = config.Type
		}
		if config.Location != nil && config.Location.ProjectKey != "" {
			projectKey = config.Location.ProjectKey
		}
	}

	if projectKey == "" {
		return nil, fmt.Errorf("cannot build local board view: board %d is not configured in local cache and no project_key was specified", boardID)
	}

	view := &jira.BoardView{
		BoardID:    boardID,
		BoardName:  boardName,
		BoardType:  boardType,
		ProjectKey: projectKey,
		Columns:    []jira.BoardColumnView{},
		Backlog:    []jira.BoardCard{},
	}

	// Status to column index map
	statusToCol := make(map[string]int)
	if config != nil && len(config.ColumnConfig.Columns) > 0 {
		for i, col := range config.ColumnConfig.Columns {
			view.Columns = append(view.Columns, jira.BoardColumnView{
				Name:  col.Name,
				Cards: []jira.BoardCard{},
				Min:   col.Min,
				Max:   col.Max,
			})
			for _, st := range col.Statuses {
				statusToCol[st.ID] = i
			}
			statusToCol[strings.ToLower(col.Name)] = i
		}
	}

	// Query cached issues for this project
	issues, err := c.Search("", projectKey, "", "", "", 200)
	if err != nil {
		return nil, fmt.Errorf("loading cached issues for board: %w", err)
	}

	// If no column config was available, dynamically create columns from distinct statuses in cached issues
	if len(view.Columns) == 0 {
		var statusOrder []string
		seenStatuses := make(map[string]struct{})
		for _, iss := range issues {
			st := strings.TrimSpace(iss.Status)
			if st == "" {
				st = "Backlog"
			}
			if _, seen := seenStatuses[st]; !seen {
				seenStatuses[st] = struct{}{}
				statusOrder = append(statusOrder, st)
			}
		}
		if len(statusOrder) == 0 {
			statusOrder = []string{"To Do", "In Progress", "Done"}
		}
		for i, stName := range statusOrder {
			view.Columns = append(view.Columns, jira.BoardColumnView{
				Name:  stName,
				Cards: []jira.BoardCard{},
			})
			statusToCol[strings.ToLower(stName)] = i
		}
	}

	// Place cards into columns or backlog
	for _, iss := range issues {
		card := jira.BoardCard{
			Key:            iss.Key,
			Summary:        iss.Summary,
			IssueType:      iss.IssueType,
			Status:         iss.Status,
			StatusCategory: iss.StatusCategory,
			Priority:       iss.Priority,
			AssigneeName:   iss.AssigneeName,
			AssigneeID:     iss.AssigneeID,
			ParentKey:      iss.ParentKey,
			EpicKey:        iss.EpicKey,
			Labels:         iss.Labels,
			Components:     iss.Components,
			UpdatedAt:      iss.UpdatedAt,
		}

		colIdx := -1
		if idx, ok := statusToCol[strings.ToLower(iss.Status)]; ok {
			colIdx = idx
		}

		if colIdx >= 0 && colIdx < len(view.Columns) {
			view.Columns[colIdx].Cards = append(view.Columns[colIdx].Cards, card)
		} else {
			if strings.EqualFold(iss.Status, "backlog") {
				if includeBacklog {
					view.Backlog = append(view.Backlog, card)
				}
			} else {
				unmappedIdx := -1
				for i, col := range view.Columns {
					if col.Name == "Unmapped" {
						unmappedIdx = i
						break
					}
				}
				if unmappedIdx == -1 {
					view.Columns = append(view.Columns, jira.BoardColumnView{
						Name:  "Unmapped",
						Cards: []jira.BoardCard{card},
					})
				} else {
					view.Columns[unmappedIdx].Cards = append(view.Columns[unmappedIdx].Cards, card)
				}
			}
		}
	}

	total := len(view.Backlog)
	for _, col := range view.Columns {
		total += len(col.Cards)
	}
	view.TotalCards = total

	return view, nil
}
