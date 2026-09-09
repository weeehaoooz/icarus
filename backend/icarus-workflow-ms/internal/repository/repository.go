package repository

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"icarus-workflow-ms/internal/models"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed migrations
var migrationsFS embed.FS

type SQLRepository struct {
	db *sql.DB
}

func NewSQLRepository(db *sql.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

// InitDB initialises SQLite and runs all pending up migrations in sequence.
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	_, _ = db.Exec("PRAGMA foreign_keys = ON;")
	_, _ = db.Exec("PRAGMA busy_timeout = 5000;")

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		db.Close()
		return nil, err
	}

	entries, err := migrationsFS.ReadDir("migrations/sqlite")
	if err != nil {
		db.Close()
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		version := entry.Name()

		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
			db.Close()
			return nil, err
		}
		if count > 0 {
			continue
		}

		upSQL, err := migrationsFS.ReadFile("migrations/sqlite/" + version)
		if err != nil {
			db.Close()
			return nil, err
		}

		for _, stmt := range strings.Split(string(upSQL), ";") {
			if trimmed := strings.TrimSpace(stmt); trimmed != "" {
				if _, err := db.Exec(trimmed); err != nil {
					db.Close()
					return nil, err
				}
			}
		}

		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			db.Close()
			return nil, err
		}
	}

	return db, nil
}

func parseTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
	}
	var lastErr error
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

// ========== WORKFLOW DEFINITIONS ==========

func (r *SQLRepository) GetWorkflowByRoleID(roleID string) (*models.Workflow, error) {
	var def models.Workflow
	var createdAtStr string
	var supersedesID sql.NullString

	err := r.db.QueryRow(`
		SELECT w.id, w.name, w.definition_key, w.version, w.is_current, w.status,
		       w.supersedes_id, w.created_by, w.created_at
		FROM workflows w
		JOIN role_workflow_mappings rwm ON rwm.workflow_id = w.id
		WHERE rwm.role_id = ? AND rwm.is_active = 1
		LIMIT 1`, roleID).
		Scan(&def.ID, &def.Name, &def.DefinitionKey, &def.Version, &def.IsCurrent,
			&def.Status, &supersedesID, &def.CreatedBy, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // no mapping
		}
		return nil, err
	}
	if supersedesID.Valid {
		def.SupersedesID = &supersedesID.String
	}
	def.CreatedAt, _ = parseTime(createdAtStr)

	nodes, err := r.getNodesForWorkflow(def.ID)
	if err != nil {
		return nil, err
	}
	def.Nodes = nodes
	return &def, nil
}

func (r *SQLRepository) GetWorkflowByID(id string) (*models.Workflow, error) {
	var def models.Workflow
	var createdAtStr string
	var supersedesID sql.NullString

	err := r.db.QueryRow(`
		SELECT id, name, definition_key, version, is_current, status,
		       supersedes_id, created_by, created_at
		FROM workflows WHERE id = ?`, id).
		Scan(&def.ID, &def.Name, &def.DefinitionKey, &def.Version, &def.IsCurrent,
			&def.Status, &supersedesID, &def.CreatedBy, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if supersedesID.Valid {
		def.SupersedesID = &supersedesID.String
	}
	def.CreatedAt, _ = parseTime(createdAtStr)

	nodes, err := r.getNodesForWorkflow(def.ID)
	if err != nil {
		return nil, err
	}
	def.Nodes = nodes
	return &def, nil
}

func (r *SQLRepository) getNodesForWorkflow(workflowID string) ([]models.WorkflowNode, error) {
	rows, err := r.db.Query(`
		SELECT id, workflow_id, name, type, depends_on, max_retries, retry_interval, config
		FROM workflow_nodes
		WHERE workflow_id = ?`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

func (r *SQLRepository) getNodesForWorkflowTx(tx *sql.Tx, workflowID string) ([]models.WorkflowNode, error) {
	rows, err := tx.Query(`
		SELECT id, workflow_id, name, type, depends_on, max_retries, retry_interval, config
		FROM workflow_nodes
		WHERE workflow_id = ?`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

func scanNodes(rows *sql.Rows) ([]models.WorkflowNode, error) {
	var nodes []models.WorkflowNode
	for rows.Next() {
		var n models.WorkflowNode
		var depStr, configStr sql.NullString
		if err := rows.Scan(&n.ID, &n.WorkflowID, &n.Name, &n.Type, &depStr, &n.MaxRetries, &n.RetryInterval, &configStr); err != nil {
			return nil, err
		}
		if depStr.Valid && depStr.String != "" {
			_ = json.Unmarshal([]byte(depStr.String), &n.DependsOn)
		} else {
			n.DependsOn = []string{}
		}
		if configStr.Valid && configStr.String != "" {
			_ = json.Unmarshal([]byte(configStr.String), &n.Config)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (r *SQLRepository) UpsertWorkflow(roleID, definitionKey, newID, createdBy string,
	req models.UpsertWorkflowRequest) (*models.Workflow, error) {

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Find current version number and existing mapping
	var oldDefID sql.NullString
	var oldVersion int
	_ = tx.QueryRow(`
		SELECT w.id, w.version
		FROM workflows w
		JOIN role_workflow_mappings rwm ON rwm.workflow_id = w.id
		WHERE rwm.role_id = ? AND rwm.is_active = 1
		LIMIT 1`, roleID).Scan(&oldDefID, &oldVersion)

	newVersion := oldVersion + 1

	// Mark old definition superseded
	var supersedesVal interface{}
	if oldDefID.Valid {
		supersedesVal = oldDefID.String
		if _, err := tx.Exec(`
			UPDATE workflows SET is_current = 0, status = 'SUPERSEDED'
			WHERE id = ?`, oldDefID.String); err != nil {
			return nil, err
		}
	}

	// Insert new definition
	if _, err := tx.Exec(`
		INSERT INTO workflows (id, name, definition_key, version, is_current, status, supersedes_id, created_by)
		VALUES (?, ?, ?, ?, 1, 'ACTIVE', ?, ?)`,
		newID, req.Name, definitionKey, newVersion, supersedesVal, createdBy); err != nil {
		return nil, err
	}

	// Insert nodes
	for _, nReq := range req.Nodes {
		depJSON, _ := json.Marshal(nReq.DependsOn)
		configJSON, _ := json.Marshal(nReq.Config)

		if _, err := tx.Exec(`
			INSERT INTO workflow_nodes
			(id, workflow_id, name, type, depends_on, max_retries, retry_interval, config)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			nReq.ID, newID, nReq.Name, nReq.Type, string(depJSON), nReq.MaxRetries, nReq.RetryInterval, string(configJSON)); err != nil {
			return nil, err
		}
	}

	// Upsert role mapping
	mappingID := "rwm-" + roleID
	if _, err := tx.Exec(`
		INSERT INTO role_workflow_mappings (id, role_id, workflow_id, is_active, created_by)
		VALUES (?, ?, ?, 1, ?)
		ON CONFLICT(role_id) DO UPDATE SET
			workflow_id = excluded.workflow_id,
			is_active = 1,
			effective_from = CURRENT_TIMESTAMP,
			effective_to = NULL`,
		mappingID, roleID, newID, createdBy); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetWorkflowByID(newID)
}

func (r *SQLRepository) ListWorkflowDefinitions() ([]models.Workflow, error) {
	rows, err := r.db.Query(`
		SELECT id, name, COALESCE(description,''), definition_key, version, is_current, status, created_by, created_at
		FROM workflows
		ORDER BY definition_key ASC, version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Workflow
	for rows.Next() {
		var w models.Workflow
		var createdAtStr string
		if err := rows.Scan(&w.ID, &w.Name, &w.Description, &w.DefinitionKey, &w.Version, &w.IsCurrent, &w.Status, &w.CreatedBy, &createdAtStr); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = parseTime(createdAtStr)
		list = append(list, w)
	}
	return list, nil
}

func (r *SQLRepository) GetDefinitionHistory(definitionKey string) ([]models.Workflow, error) {
	rows, err := r.db.Query(`
		SELECT id, name, COALESCE(description,''), definition_key, version, is_current, status, created_by, created_at
		FROM workflows
		WHERE definition_key = ?
		ORDER BY version DESC`, definitionKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Workflow
	for rows.Next() {
		var w models.Workflow
		var createdAtStr string
		if err := rows.Scan(&w.ID, &w.Name, &w.Description, &w.DefinitionKey, &w.Version, &w.IsCurrent, &w.Status, &w.CreatedBy, &createdAtStr); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = parseTime(createdAtStr)
		list = append(list, w)
	}
	return list, nil
}

// CreateStandaloneWorkflow creates a reusable workflow template not yet bound to any role.
func (r *SQLRepository) CreateStandaloneWorkflow(createdBy string, req models.UpsertWorkflowRequest) (*models.Workflow, error) {
	newID := uuid.New().String()
	definitionKey := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check for existing current definition with same key and supersede it
	var oldID sql.NullString
	var oldVersion int
	_ = tx.QueryRow(`SELECT id, version FROM workflows WHERE definition_key = ? AND is_current = 1`, definitionKey).
		Scan(&oldID, &oldVersion)

	newVersion := oldVersion + 1
	var supersedesVal interface{}
	if oldID.Valid {
		supersedesVal = oldID.String
		if _, err := tx.Exec(`UPDATE workflows SET is_current = 0, status = 'SUPERSEDED' WHERE id = ?`, oldID.String); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`UPDATE role_workflow_mappings SET workflow_id = ? WHERE workflow_id = ?`, newID, oldID.String); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO workflows (id, name, description, definition_key, version, is_current, status, supersedes_id, created_by)
		VALUES (?, ?, ?, ?, ?, 1, 'ACTIVE', ?, ?)`,
		newID, req.Name, req.Description, definitionKey, newVersion, supersedesVal, createdBy); err != nil {
		return nil, err
	}

	for _, nReq := range req.Nodes {
		depJSON, _ := json.Marshal(nReq.DependsOn)
		configJSON, _ := json.Marshal(nReq.Config)
		if _, err := tx.Exec(`
			INSERT INTO workflow_nodes (id, workflow_id, name, type, depends_on, max_retries, retry_interval, config)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			nReq.ID, newID, nReq.Name, nReq.Type, string(depJSON), nReq.MaxRetries, nReq.RetryInterval, string(configJSON)); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetWorkflowByID(newID)
}

// MapWorkflowToRole points a role at an existing workflow template (by workflow UUID).
// This is non-destructive to the template itself — only the mapping is updated.
func (r *SQLRepository) MapWorkflowToRole(roleID, workflowID, createdBy string) error {
	mappingID := "rwm-" + roleID
	_, err := r.db.Exec(`
		INSERT INTO role_workflow_mappings (id, role_id, workflow_id, is_active, created_by)
		VALUES (?, ?, ?, 1, ?)
		ON CONFLICT(role_id) DO UPDATE SET
			workflow_id = excluded.workflow_id,
			is_active = 1,
			effective_from = CURRENT_TIMESTAMP,
			effective_to = NULL`,
		mappingID, roleID, workflowID, createdBy)
	return err
}

// UnmapWorkflowFromRole deactivates the role→workflow mapping (sets is_active=0) without deleting the record.
func (r *SQLRepository) UnmapWorkflowFromRole(roleID string) error {
	res, err := r.db.Exec(`
		UPDATE role_workflow_mappings
		SET is_active = 0, effective_to = CURRENT_TIMESTAMP
		WHERE role_id = ? AND is_active = 1`, roleID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("no active mapping found for role")
	}
	return nil
}

// UnmapRoleFromWorkflow deactivates a specific role→workflow mapping identified by both workflow ID and role ID.
func (r *SQLRepository) UnmapRoleFromWorkflow(workflowID, roleID string) error {
	res, err := r.db.Exec(`
		UPDATE role_workflow_mappings
		SET is_active = 0, effective_to = CURRENT_TIMESTAMP
		WHERE workflow_id = ? AND role_id = ? AND is_active = 1`, workflowID, roleID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("no active mapping found for this workflow/role combination")
	}
	return nil
}

// ListWorkflowsWithRoleMapping returns all current workflow versions annotated with which role (if any) references them.
func (r *SQLRepository) ListWorkflowsWithRoleMapping() ([]models.WorkflowWithRoleMapping, error) {
	rows, err := r.db.Query(`
		SELECT w.id, w.name, COALESCE(w.description,''), w.definition_key, w.version, w.is_current, w.status, w.created_by, w.created_at,
		       COALESCE(group_concat(rwm.role_id, ','), '') as mapped_roles
		FROM workflows w
		LEFT JOIN role_workflow_mappings rwm ON rwm.workflow_id = w.id AND rwm.is_active = 1
		WHERE w.is_current = 1
		GROUP BY w.id
		ORDER BY w.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.WorkflowWithRoleMapping
	for rows.Next() {
		var wm models.WorkflowWithRoleMapping
		var createdAtStr string
		var mappedRolesStr string
		if err := rows.Scan(&wm.ID, &wm.Name, &wm.Description, &wm.DefinitionKey,
			&wm.Version, &wm.IsCurrent, &wm.Status, &wm.CreatedBy, &createdAtStr, &mappedRolesStr); err != nil {
			return nil, err
		}
		wm.CreatedAt, _ = parseTime(createdAtStr)
		if mappedRolesStr != "" {
			wm.MappedRoleIDs = strings.Split(mappedRolesStr, ",")
		} else {
			wm.MappedRoleIDs = []string{}
		}
		list = append(list, wm)
	}
	return list, nil
}

// ========== CARTS ==========

func (r *SQLRepository) CreateCart(cart *models.AccessCart) error {
	_, err := r.db.Exec(`
		INSERT INTO access_carts (id, requester_id, status, justification)
		VALUES (?, ?, ?, ?)`,
		cart.ID, cart.RequesterID, cart.Status, cart.Justification)
	return err
}

func (r *SQLRepository) GetCart(cartID string) (*models.AccessCart, error) {
	var c models.AccessCart
	var createdAtStr, updatedAtStr string
	var submittedAt, completedAt sql.NullString

	err := r.db.QueryRow(`
		SELECT id, requester_id, status, justification, submitted_at, completed_at,
		       version, created_at, updated_at
		FROM access_carts WHERE id = ?`, cartID).
		Scan(&c.ID, &c.RequesterID, &c.Status, &c.Justification,
			&submittedAt, &completedAt, &c.Version, &createdAtStr, &updatedAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("cart not found")
		}
		return nil, err
	}

	c.CreatedAt, _ = parseTime(createdAtStr)
	c.UpdatedAt, _ = parseTime(updatedAtStr)
	if submittedAt.Valid {
		t, _ := parseTime(submittedAt.String)
		c.SubmittedAt = &t
	}
	if completedAt.Valid {
		t, _ := parseTime(completedAt.String)
		c.CompletedAt = &t
	}

	items, err := r.GetCartItems(cartID)
	if err != nil {
		return nil, err
	}
	c.Items = items

	if c.Status == "SUBMITTED" || c.Status == "IN_PROGRESS" {
		steps, err := r.GetPendingStepsForCart(cartID)
		if err == nil {
			c.PendingSteps = steps
		}
	}

	return &c, nil
}

func (r *SQLRepository) ListCarts(requesterID string, includeArchived bool) ([]models.AccessCart, error) {
	var query string
	if includeArchived {
		query = `
			SELECT id, requester_id, status, justification, submitted_at, completed_at,
			       version, created_at, updated_at
			FROM access_carts WHERE requester_id = ?
			ORDER BY created_at DESC`
	} else {
		query = `
			SELECT id, requester_id, status, justification, submitted_at, completed_at,
			       version, created_at, updated_at
			FROM access_carts WHERE requester_id = ? AND status != 'ARCHIVED'
			ORDER BY created_at DESC`
	}
	rows, err := r.db.Query(query, requesterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.AccessCart
	for rows.Next() {
		var c models.AccessCart
		var createdAtStr, updatedAtStr string
		var submittedAt, completedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.RequesterID, &c.Status, &c.Justification,
			&submittedAt, &completedAt, &c.Version, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = parseTime(createdAtStr)
		c.UpdatedAt, _ = parseTime(updatedAtStr)
		if submittedAt.Valid {
			t, _ := parseTime(submittedAt.String)
			c.SubmittedAt = &t
		}
		if completedAt.Valid {
			t, _ := parseTime(completedAt.String)
			c.CompletedAt = &t
		}

		items, err := r.GetCartItems(c.ID)
		if err != nil {
			return nil, err
		}
		c.Items = items

		if c.Status == "SUBMITTED" || c.Status == "IN_PROGRESS" {
			steps, err := r.GetPendingStepsForCart(c.ID)
			if err == nil {
				c.PendingSteps = steps
			}
		}

		list = append(list, c)
	}
	return list, nil
}

func (r *SQLRepository) ListAllCarts() ([]models.AccessCart, error) {
	rows, err := r.db.Query(`
		SELECT id, requester_id, status, justification, submitted_at, completed_at,
		       version, created_at, updated_at
		FROM access_carts
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.AccessCart
	for rows.Next() {
		var c models.AccessCart
		var createdAtStr, updatedAtStr string
		var submittedAt, completedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.RequesterID, &c.Status, &c.Justification,
			&submittedAt, &completedAt, &c.Version, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = parseTime(createdAtStr)
		c.UpdatedAt, _ = parseTime(updatedAtStr)
		if submittedAt.Valid {
			t, _ := parseTime(submittedAt.String)
			c.SubmittedAt = &t
		}
		if completedAt.Valid {
			t, _ := parseTime(completedAt.String)
			c.CompletedAt = &t
		}

		items, err := r.GetCartItems(c.ID)
		if err != nil {
			return nil, err
		}
		c.Items = items

		if c.Status == "SUBMITTED" || c.Status == "IN_PROGRESS" {
			steps, err := r.GetPendingStepsForCart(c.ID)
			if err == nil {
				c.PendingSteps = steps
			}
		}

		list = append(list, c)
	}
	return list, nil
}

func (r *SQLRepository) ArchiveClosedCarts(olderThanDays int) (int64, error) {
	var res sql.Result
	var err error
	if olderThanDays <= 0 {
		tx, err := r.db.Begin()
		if err != nil {
			return 0, err
		}
		defer tx.Rollback()

		res, err = tx.Exec(`
			UPDATE access_carts 
			SET status = 'ARCHIVED', updated_at = CURRENT_TIMESTAMP 
			WHERE status IN ('COMPLETED', 'CANCELLED')`)
		if err != nil {
			return 0, err
		}

		rowsAffected, _ := res.RowsAffected()

		if rowsAffected > 0 {
			_, err = tx.Exec(`
				UPDATE access_cart_items 
				SET status = 'ARCHIVED', updated_at = CURRENT_TIMESTAMP 
				WHERE cart_id IN (
					SELECT id FROM access_carts WHERE status = 'ARCHIVED'
				) AND status IN ('APPROVED', 'REJECTED', 'CANCELLED')`)
			if err != nil {
				return 0, err
			}
		}

		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return rowsAffected, nil
	}

	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	cutoffStr := cutoff.UTC().Format("2006-01-02 15:04:05")

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err = tx.Exec(`
		UPDATE access_carts 
		SET status = 'ARCHIVED', updated_at = CURRENT_TIMESTAMP 
		WHERE status IN ('COMPLETED', 'CANCELLED') AND completed_at < ?`, cutoffStr)
	if err != nil {
		return 0, err
	}

	rowsAffected, _ := res.RowsAffected()

	if rowsAffected > 0 {
		_, err = tx.Exec(`
			UPDATE access_cart_items 
			SET status = 'ARCHIVED', updated_at = CURRENT_TIMESTAMP 
			WHERE cart_id IN (
				SELECT id FROM access_carts WHERE status = 'ARCHIVED'
			) AND status IN ('APPROVED', 'REJECTED', 'CANCELLED')`)
		if err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

func (r *SQLRepository) ArchiveSelectedCarts(status string, olderThanDays int, ids []string) (int64, error) {
	var targetIDs []interface{}

	if len(ids) > 0 {
		for _, id := range ids {
			targetIDs = append(targetIDs, id)
		}
	} else {
		// Select based on criteria
		var query string
		var args []interface{}
		query = `SELECT id FROM access_carts WHERE 1=1`

		if status == "ALL" {
			query += ` AND status IN ('COMPLETED', 'CANCELLED')`
		} else {
			query += ` AND status = ?`
			args = append(args, status)
		}

		if olderThanDays > 0 {
			cutoff := time.Now().AddDate(0, 0, -olderThanDays)
			cutoffStr := cutoff.UTC().Format("2006-01-02 15:04:05")
			query += ` AND COALESCE(completed_at, created_at) < ?`
			args = append(args, cutoffStr)
		}

		rows, err := r.db.Query(query, args...)
		if err != nil {
			return 0, err
		}
		defer rows.Close()

		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return 0, err
			}
			targetIDs = append(targetIDs, id)
		}
	}

	if len(targetIDs) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(targetIDs))
	for i := range targetIDs {
		placeholders[i] = "?"
	}
	inClause := strings.Join(placeholders, ",")

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Update access_carts
	queryCarts := fmt.Sprintf(`
		UPDATE access_carts 
		SET status = 'ARCHIVED', updated_at = CURRENT_TIMESTAMP 
		WHERE id IN (%s)`, inClause)
	res, err := tx.Exec(queryCarts, targetIDs...)
	if err != nil {
		return 0, err
	}

	rowsAffected, _ := res.RowsAffected()

	// Update access_cart_items
	queryItems := fmt.Sprintf(`
		UPDATE access_cart_items 
		SET status = 'ARCHIVED', updated_at = CURRENT_TIMESTAMP 
		WHERE cart_id IN (%s) AND status IN ('APPROVED', 'REJECTED', 'CANCELLED', 'PENDING')`, inClause)
	_, err = tx.Exec(queryItems, targetIDs...)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

func (r *SQLRepository) DeleteSelectedCarts(status string, olderThanDays int, includeDrafts bool, ids []string) (int64, error) {
	var targetIDs []interface{}

	if len(ids) > 0 {
		for _, id := range ids {
			targetIDs = append(targetIDs, id)
		}
	} else {
		// Select based on criteria
		var query string
		var args []interface{}
		query = `SELECT id FROM access_carts WHERE 1=1`

		if status == "ALL" {
			if includeDrafts {
				query += ` AND status IN ('COMPLETED', 'CANCELLED', 'ARCHIVED', 'DRAFT')`
			} else {
				query += ` AND status IN ('COMPLETED', 'CANCELLED', 'ARCHIVED')`
			}
		} else {
			query += ` AND status = ?`
			args = append(args, status)
		}

		if olderThanDays > 0 {
			cutoff := time.Now().AddDate(0, 0, -olderThanDays)
			cutoffStr := cutoff.UTC().Format("2006-01-02 15:04:05")
			query += ` AND COALESCE(completed_at, created_at) < ?`
			args = append(args, cutoffStr)
		}

		rows, err := r.db.Query(query, args...)
		if err != nil {
			return 0, err
		}
		defer rows.Close()

		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return 0, err
			}
			targetIDs = append(targetIDs, id)
		}
	}

	if len(targetIDs) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(targetIDs))
	for i := range targetIDs {
		placeholders[i] = "?"
	}
	inClause := strings.Join(placeholders, ",")

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 1. Delete execution_nodes
	querySteps := fmt.Sprintf(`
		DELETE FROM execution_nodes 
		WHERE execution_id IN (
			SELECT id FROM executions 
			WHERE cart_item_id IN (
				SELECT id FROM access_cart_items 
				WHERE cart_id IN (%s)
			)
		)`, inClause)
	_, err = tx.Exec(querySteps, targetIDs...)
	if err != nil {
		return 0, err
	}

	// 2. Delete executions
	queryInstances := fmt.Sprintf(`
		DELETE FROM executions 
		WHERE cart_item_id IN (
			SELECT id FROM access_cart_items 
			WHERE cart_id IN (%s)
		)`, inClause)
	_, err = tx.Exec(queryInstances, targetIDs...)
	if err != nil {
		return 0, err
	}

	// 3. Delete access_cart_items
	queryItems := fmt.Sprintf(`
		DELETE FROM access_cart_items WHERE cart_id IN (%s)`, inClause)
	_, err = tx.Exec(queryItems, targetIDs...)
	if err != nil {
		return 0, err
	}

	// 4. Delete access_carts
	queryCarts := fmt.Sprintf(`
		DELETE FROM access_carts WHERE id IN (%s)`, inClause)
	res, err := tx.Exec(queryCarts, targetIDs...)
	if err != nil {
		return 0, err
	}

	rowsAffected, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return rowsAffected, nil
}

func (r *SQLRepository) UpdateCartStatus(cartID, status string) error {
	_, err := r.db.Exec(`
		UPDATE access_carts SET status = ?, updated_at = CURRENT_TIMESTAMP,
		submitted_at = CASE WHEN ? = 'SUBMITTED' THEN CURRENT_TIMESTAMP ELSE submitted_at END,
		completed_at = CASE WHEN ? IN ('COMPLETED', 'CANCELLED') THEN CURRENT_TIMESTAMP ELSE completed_at END
		WHERE id = ?`, status, status, status, cartID)
	return err
}

func (r *SQLRepository) UpdateCartJustification(cartID, justification string) error {
	_, err := r.db.Exec(`
		UPDATE access_carts SET justification = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, justification, cartID)
	return err
}

// ========== CART ITEMS ==========

func (r *SQLRepository) AddCartItem(item *models.CartItem) error {
	_, err := r.db.Exec(`
		INSERT INTO access_cart_items (id, cart_id, role_id, role_name, status)
		VALUES (?, ?, ?, ?, 'PENDING')`,
		item.ID, item.CartID, item.RoleID, item.RoleName)
	return err
}

func (r *SQLRepository) GetCartItems(cartID string) ([]models.CartItem, error) {
	rows, err := r.db.Query(`
		SELECT id, cart_id, role_id, role_name, status, execution_id, created_at, updated_at
		FROM access_cart_items WHERE cart_id = ?`, cartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.CartItem
	for rows.Next() {
		var item models.CartItem
		var createdAtStr, updatedAtStr string
		var execID sql.NullString
		if err := rows.Scan(&item.ID, &item.CartID, &item.RoleID, &item.RoleName,
			&item.Status, &execID, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		if execID.Valid {
			item.ExecutionID = &execID.String
		}
		item.CreatedAt, _ = parseTime(createdAtStr)
		item.UpdatedAt, _ = parseTime(updatedAtStr)
		list = append(list, item)
	}
	return list, nil
}

func (r *SQLRepository) GetCartItem(itemID string) (*models.CartItem, error) {
	var item models.CartItem
	var createdAtStr, updatedAtStr string
	var execID sql.NullString
	err := r.db.QueryRow(`
		SELECT id, cart_id, role_id, role_name, status, execution_id, created_at, updated_at
		FROM access_cart_items WHERE id = ?`, itemID).
		Scan(&item.ID, &item.CartID, &item.RoleID, &item.RoleName,
			&item.Status, &execID, &createdAtStr, &updatedAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("cart item not found")
		}
		return nil, err
	}
	if execID.Valid {
		item.ExecutionID = &execID.String
	}
	item.CreatedAt, _ = parseTime(createdAtStr)
	item.UpdatedAt, _ = parseTime(updatedAtStr)
	return &item, nil
}

func (r *SQLRepository) RemoveCartItem(itemID, cartID string) error {
	res, err := r.db.Exec(`DELETE FROM access_cart_items WHERE id = ? AND cart_id = ?`, itemID, cartID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("item not found in cart")
	}
	return nil
}

func (r *SQLRepository) UpdateCartItemStatus(itemID, status string, executionID *string) error {
	_, err := r.db.Exec(`
		UPDATE access_cart_items SET status = ?, execution_id = COALESCE(?, execution_id),
		updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, executionID, itemID)
	return err
}

func (r *SQLRepository) CheckAllCartItemsTerminal(cartID string) (bool, string, error) {
	rows, err := r.db.Query(`SELECT status FROM access_cart_items WHERE cart_id = ?`, cartID)
	if err != nil {
		return false, "", err
	}
	defer rows.Close()

	allDone := true
	overallStatus := "COMPLETED"
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return false, "", err
		}
		if s != "APPROVED" && s != "REJECTED" && s != "CANCELLED" {
			allDone = false
			break
		}
		if s == "REJECTED" {
			overallStatus = "COMPLETED"
		}
	}
	return allDone, overallStatus, nil
}

// ========== RUNS (EXECUTIONS) ==========

func (r *SQLRepository) CreateWorkflowInstance(exec *models.Execution) error {
	_, err := r.db.Exec(`
		INSERT INTO executions (id, workflow_id, cart_item_id, status, error_message)
		VALUES (?, ?, ?, 'IN_PROGRESS', ?)`,
		exec.ID, exec.WorkflowID, exec.CartItemID, exec.ErrorMessage)
	return err
}

func (r *SQLRepository) StartExecutionAndProgress(exec *models.Execution, actorUserID string) (
	newSteps []models.ExecutionNode,
	err error,
) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO executions (id, workflow_id, cart_item_id, status, error_message)
		VALUES (?, ?, ?, 'IN_PROGRESS', ?)`,
		exec.ID, exec.WorkflowID, exec.CartItemID, exec.ErrorMessage)
	if err != nil {
		return nil, err
	}

	_, newSteps, _, err = r.ProgressExecutionTx(tx, exec.ID, actorUserID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return newSteps, nil
}

func (r *SQLRepository) GetWorkflowInstance(executionID string) (*models.Execution, error) {
	var exec models.Execution
	var startedAtStr string
	var completedAt sql.NullString
	var errMsg sql.NullString

	err := r.db.QueryRow(`
		SELECT id, workflow_id, cart_item_id, status, error_message, started_at, completed_at, version
		FROM executions WHERE id = ?`, executionID).
		Scan(&exec.ID, &exec.WorkflowID, &exec.CartItemID, &exec.Status, &errMsg, &startedAtStr, &completedAt, &exec.Version)
	if err != nil {
		return nil, err
	}
	if errMsg.Valid {
		exec.ErrorMessage = &errMsg.String
	}
	exec.StartedAt, _ = parseTime(startedAtStr)
	if completedAt.Valid {
		t, _ := parseTime(completedAt.String)
		exec.CompletedAt = &t
	}
	return &exec, nil
}

func (r *SQLRepository) UpdateExecutionStatus(execID, status string) error {
	_, err := r.db.Exec(`
		UPDATE executions SET status = ?, completed_at = CASE WHEN ? IN ('COMPLETED', 'REJECTED', 'CANCELLED') THEN CURRENT_TIMESTAMP ELSE NULL END, version = version + 1
		WHERE id = ?`, status, status, execID)
	return err
}

func (r *SQLRepository) UpdateExecutionError(execID string, errMsg string) error {
	_, err := r.db.Exec(`
		UPDATE executions SET error_message = ?, version = version + 1
		WHERE id = ?`, errMsg, execID)
	return err
}

func (r *SQLRepository) CreateWorkflowStep(node *models.ExecutionNode) error {
	_, err := r.db.Exec(`
		INSERT INTO execution_nodes
		(id, execution_id, node_id, node_name, assigned_to_user_id, assigned_to_role, status, retry_attempt)
		VALUES (?, ?, ?, ?, ?, ?, 'PENDING', ?)`,
		node.ID, node.ExecutionID, node.NodeID, node.NodeName, node.AssignedToUserID, node.AssignedToRole, node.RetryAttempt)
	return err
}

func (r *SQLRepository) GetWorkflowStep(nodeID string) (*models.ExecutionNode, error) {
	var node models.ExecutionNode
	var assignedAtStr string
	var actedAt sql.NullString
	var u, ro, ab sql.NullString

	err := r.db.QueryRow(`
		SELECT id, execution_id, node_id, node_name, assigned_to_user_id, assigned_to_role,
		       status, COALESCE(decision_comment, ''), acted_by_user_id, assigned_at, acted_at, retry_attempt, version
		FROM execution_nodes WHERE id = ?`, nodeID).
		Scan(&node.ID, &node.ExecutionID, &node.NodeID, &node.NodeName, &u, &ro,
			&node.Status, &node.DecisionComment, &ab, &assignedAtStr, &actedAt, &node.RetryAttempt, &node.Version)
	if err != nil {
		return nil, err
	}
	if u.Valid {
		node.AssignedToUserID = &u.String
	}
	if ro.Valid {
		node.AssignedToRole = &ro.String
	}
	if ab.Valid {
		node.ActedByUserID = &ab.String
	}
	node.AssignedAt, _ = parseTime(assignedAtStr)
	if actedAt.Valid {
		t, _ := parseTime(actedAt.String)
		node.ActedAt = &t
	}
	return &node, nil
}

// ========== PROGRESSION ENGINE ==========

func (r *SQLRepository) ProgressExecutionTx(tx *sql.Tx, execID string, actorUserID string) (
	exec *models.Execution,
	newSteps []models.ExecutionNode,
	msg string,
	err error,
) {
	// Load execution
	exec = &models.Execution{}
	var startedAtStr string
	var completedAt sql.NullString
	var errMsg sql.NullString
	err = tx.QueryRow(`
		SELECT id, workflow_id, cart_item_id, status, error_message, started_at, completed_at, version
		FROM executions WHERE id = ?`, execID).
		Scan(&exec.ID, &exec.WorkflowID, &exec.CartItemID, &exec.Status, &errMsg, &startedAtStr, &completedAt, &exec.Version)
	if err != nil {
		return nil, nil, "", err
	}
	if errMsg.Valid {
		exec.ErrorMessage = &errMsg.String
	}
	exec.StartedAt, _ = parseTime(startedAtStr)
	if completedAt.Valid {
		t, _ := parseTime(completedAt.String)
		exec.CompletedAt = &t
	}

	// Load workflow and nodes
	var wf models.Workflow
	err = tx.QueryRow(`SELECT id, name, definition_key, version FROM workflows WHERE id = ?`, exec.WorkflowID).
		Scan(&wf.ID, &wf.Name, &wf.DefinitionKey, &wf.Version)
	if err != nil {
		return nil, nil, "", err
	}
	wf.Nodes, err = r.getNodesForWorkflowTx(tx, wf.ID)
	if err != nil {
		return nil, nil, "", err
	}

	correlationID := uuid.New().String()

	for {
		// 1. Load all execution nodes for this execution so far
		rows, err := tx.Query(`
			SELECT id, execution_id, node_id, node_name, assigned_to_user_id, assigned_to_role, status, COALESCE(decision_comment, ''), acted_by_user_id, assigned_at, acted_at, retry_attempt, version
			FROM execution_nodes WHERE execution_id = ?`, exec.ID)
		if err != nil {
			return nil, nil, "", err
		}

		var steps []models.ExecutionNode
		for rows.Next() {
			var step models.ExecutionNode
			var assignedAtStr string
			var actedAt sql.NullString
			var u, ro, ab sql.NullString
			if err := rows.Scan(&step.ID, &step.ExecutionID, &step.NodeID, &step.NodeName, &u, &ro, &step.Status, &step.DecisionComment, &ab, &assignedAtStr, &actedAt, &step.RetryAttempt, &step.Version); err != nil {
				rows.Close()
				return nil, nil, "", err
			}
			if u.Valid {
				step.AssignedToUserID = &u.String
			}
			if ro.Valid {
				step.AssignedToRole = &ro.String
			}
			if ab.Valid {
				step.ActedByUserID = &ab.String
			}
			step.AssignedAt, _ = parseTime(assignedAtStr)
			if actedAt.Valid {
				t, _ := parseTime(actedAt.String)
				step.ActedAt = &t
			}
			steps = append(steps, step)
		}
		rows.Close()

		// Group steps by node ID
		stepsMap := make(map[string][]models.ExecutionNode)
		for _, s := range steps {
			stepsMap[s.NodeID] = append(stepsMap[s.NodeID], s)
		}

		// 2. Evaluate status of each workflow node
		nodeStatuses := make(map[string]NodeStatus)
		for _, node := range wf.Nodes {
			nodeStatuses[node.ID] = EvaluateNodeStatus(node, stepsMap[node.ID])
		}

		// 3. Check if any node is REJECTED
		hasRejections := false
		var rejectedNodeID string
		for nodeID, status := range nodeStatuses {
			if status == NodeStatusRejected {
				hasRejections = true
				rejectedNodeID = nodeID
				break
			}
		}

		if hasRejections {
			// Update execution status to REJECTED
			_, err = tx.Exec(`
				UPDATE executions SET status = 'REJECTED', completed_at = CURRENT_TIMESTAMP, version = version + 1
				WHERE id = ?`, exec.ID)
			if err != nil {
				return nil, nil, "", err
			}
			exec.Status = "REJECTED"

			// Cancel remaining PENDING execution nodes
			_, err = tx.Exec(`
				UPDATE execution_nodes SET status = 'CANCELLED', version = version + 1
				WHERE execution_id = ? AND status = 'PENDING'`, exec.ID)
			if err != nil {
				return nil, nil, "", err
			}

			// Update access_cart_items status
			_, err = tx.Exec(`
				UPDATE access_cart_items SET status = 'REJECTED', updated_at = CURRENT_TIMESTAMP
				WHERE id = ?`, exec.CartItemID)
			if err != nil {
				return nil, nil, "", err
			}

			// Check and update cart completion
			if err := r.checkCartCompletionTx(tx, exec.CartItemID); err != nil {
				return nil, nil, "", err
			}

			// Write audit log
			_, err = tx.Exec(`
				INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action)
				VALUES (?, 'WORKFLOW_INSTANCE', ?, ?, 'REJECTED')`,
				uuid.New().String(), exec.ID, actorUserID)
			if err != nil {
				return nil, nil, "", err
			}

			return exec, nil, fmt.Sprintf("Execution rejected at node: %s", rejectedNodeID), nil
		}

		// 4. Identify nodes ready to be activated
		nextNodesToActivate := GetNextActiveNodes(&wf, stepsMap, nodeStatuses)

		// 5. If no new nodes can be activated:
		if len(nextNodesToActivate) == 0 {
			// Check if there are any active/pending steps
			hasPendingSteps := false
			for _, step := range steps {
				if step.Status == "PENDING" {
					hasPendingSteps = true
					break
				}
			}

			if !hasPendingSteps {
				// No pending steps left AND no new nodes can be activated -> all nodes completed successfully!
				_, err = tx.Exec(`
					UPDATE executions SET status = 'COMPLETED', completed_at = CURRENT_TIMESTAMP, version = version + 1
					WHERE id = ?`, exec.ID)
				if err != nil {
					return nil, nil, "", err
				}
				exec.Status = "COMPLETED"

				// Update access_cart_items status
				_, err = tx.Exec(`
					UPDATE access_cart_items SET status = 'APPROVED', updated_at = CURRENT_TIMESTAMP
					WHERE id = ?`, exec.CartItemID)
				if err != nil {
					return nil, nil, "", err
				}

				// Check and update cart completion
				if err := r.checkCartCompletionTx(tx, exec.CartItemID); err != nil {
					return nil, nil, "", err
				}

				// Write audit log
				_, err = tx.Exec(`
					INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action)
					VALUES (?, 'WORKFLOW_INSTANCE', ?, 'system', 'COMPLETED')`,
					uuid.New().String(), exec.ID)
				if err != nil {
					return nil, nil, "", err
				}

				return exec, nil, "Workflow execution completed successfully", nil
			}

			// There are still pending human approvals
			return exec, newSteps, "Waiting for other approvals", nil
		}

		// 6. We have new nodes to activate!
		for _, node := range nextNodesToActivate {
			if node.Type == "APPROVAL" {
				candidates, err := parseCandidates(node.Config)
				if err != nil {
					// Update error message in executions table and abort progression
					_, _ = tx.Exec(`UPDATE executions SET error_message = ?, status = 'ERROR', version = version + 1 WHERE id = ?`, "Invalid candidates configuration: "+err.Error(), exec.ID)
					return exec, nil, "Invalid candidates config", nil
				}
				for _, candidate := range candidates {
					step := models.ExecutionNode{
						ID:          uuid.New().String(),
						ExecutionID: exec.ID,
						NodeID:      node.ID,
						NodeName:    node.Name,
						Status:      "PENDING",
						AssignedAt:  time.Now(),
					}
					if candidate.Type == "USER" {
						val := candidate.Value
						step.AssignedToUserID = &val
					} else if candidate.Type == "ROLE" {
						val := candidate.Value
						step.AssignedToRole = &val
					}

					_, err = tx.Exec(`
						INSERT INTO execution_nodes
						(id, execution_id, node_id, node_name, assigned_to_user_id, assigned_to_role, status, retry_attempt)
						VALUES (?, ?, ?, ?, ?, ?, 'PENDING', 0)`,
						step.ID, step.ExecutionID, step.NodeID, step.NodeName, step.AssignedToUserID, step.AssignedToRole)
					if err != nil {
						return nil, nil, "", err
					}

					// Log audit log for step assignment
					_, err = tx.Exec(`
						INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action, after_state, correlation_id)
						VALUES (?, 'WORKFLOW_STEP', ?, 'system', 'ASSIGNED', ?, ?)`,
						uuid.New().String(), step.ID, fmt.Sprintf(`{"node":"%s","node_id":"%s"}`, node.Name, node.ID), correlationID)
					if err != nil {
						return nil, nil, "", err
					}

					newSteps = append(newSteps, step)
				}
			} else {
				// Automated operation: auto-approve immediately (can support retries if we implemented error rates, etc.)
				step := models.ExecutionNode{
					ID:          uuid.New().String(),
					ExecutionID: exec.ID,
					NodeID:      node.ID,
					NodeName:    node.Name,
					Status:      "APPROVED",
					AssignedAt:  time.Now(),
				}
				_, err = tx.Exec(`
					INSERT INTO execution_nodes
					(id, execution_id, node_id, node_name, status, acted_by_user_id, acted_at)
					VALUES (?, ?, ?, ?, 'APPROVED', 'system', CURRENT_TIMESTAMP)`,
					step.ID, step.ExecutionID, step.NodeID, step.NodeName)
				if err != nil {
					return nil, nil, "", err
				}

				// Log audit log for auto execution
				_, err = tx.Exec(`
					INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action, after_state)
					VALUES (?, 'WORKFLOW_STEP', ?, 'system', 'EXECUTED', ?)`,
					uuid.New().String(), step.ID, fmt.Sprintf(`{"node":"%s","node_id":"%s"}`, node.Name, node.ID))
				if err != nil {
					return nil, nil, "", err
				}
			}
		}

		// Keep looping to evaluate and progress further nodes now that these are activated!
	}
}

func (r *SQLRepository) ActionStepAndProgress(stepID, actionStatus, comment, actorUserID string) (
	exec *models.Execution,
	nextSteps []models.ExecutionNode,
	nodeStatus NodeStatus,
	message string,
	err error,
) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, nil, "", "", err
	}
	defer tx.Rollback()

	// 1. Update the execution node status
	res, err := tx.Exec(`
		UPDATE execution_nodes SET status = ?, decision_comment = ?, acted_by_user_id = ?,
		acted_at = CURRENT_TIMESTAMP, version = version + 1
		WHERE id = ? AND status = 'PENDING'`, actionStatus, comment, actorUserID, stepID)
	if err != nil {
		return nil, nil, "", "", err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return nil, nil, "", "", errors.New("execution node is no longer pending")
	}

	// Write audit log for step action
	stepLogID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action, after_state)
		VALUES (?, 'WORKFLOW_STEP', ?, ?, ?, ?)`,
		stepLogID, stepID, actorUserID, actionStatus, fmt.Sprintf(`{"comment":"%s"}`, comment))
	if err != nil {
		return nil, nil, "", "", err
	}

	// 2. Load execution ID for this step
	var execID string
	err = tx.QueryRow(`SELECT execution_id FROM execution_nodes WHERE id = ?`, stepID).Scan(&execID)
	if err != nil {
		return nil, nil, "", "", err
	}

	// 3. Progress the execution along the graph DAG
	exec, nextSteps, message, err = r.ProgressExecutionTx(tx, execID, actorUserID)
	if err != nil {
		return nil, nil, "", "", err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, "", "", err
	}

	return exec, nextSteps, NodeStatus(exec.Status), message, nil
}

func (r *SQLRepository) DelegateStep(stepID, comment, actorUserID, delegateUserID string, newStepID string) (*models.ExecutionNode, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Update original step
	res, err := tx.Exec(`
		UPDATE execution_nodes SET status = 'DELEGATED', decision_comment = ?, acted_by_user_id = ?,
		acted_at = CURRENT_TIMESTAMP, version = version + 1
		WHERE id = ? AND status = 'PENDING'`, comment, actorUserID, stepID)
	if err != nil {
		return nil, err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return nil, errors.New("step is no longer pending")
	}

	// Get original step details
	var execID, nodeID, nodeName string
	err = tx.QueryRow(`SELECT execution_id, node_id, node_name FROM execution_nodes WHERE id = ?`, stepID).Scan(&execID, &nodeID, &nodeName)
	if err != nil {
		return nil, err
	}

	// Create new step
	newStep := &models.ExecutionNode{
		ID:               newStepID,
		ExecutionID:      execID,
		NodeID:           nodeID,
		NodeName:         nodeName,
		AssignedToUserID: &delegateUserID,
		Status:           "PENDING",
		AssignedAt:       time.Now(),
	}

	_, err = tx.Exec(`
		INSERT INTO execution_nodes
		(id, execution_id, node_id, node_name, assigned_to_user_id, status)
		VALUES (?, ?, ?, ?, ?, 'PENDING')`,
		newStep.ID, newStep.ExecutionID, newStep.NodeID, newStep.NodeName, newStep.AssignedToUserID)
	if err != nil {
		return nil, err
	}

	// Write audit log
	_, err = tx.Exec(`
		INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action, after_state)
		VALUES (?, 'WORKFLOW_STEP', ?, ?, 'DELEGATED', ?)`,
		uuid.New().String(), stepID, actorUserID, fmt.Sprintf(`{"delegated_to":"%s","new_step_id":"%s"}`, delegateUserID, newStep.ID))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return newStep, nil
}

func (r *SQLRepository) GetInboxItems(userID string, roles []string) ([]models.InboxItem, error) {
	placeholders := make([]string, len(roles))
	args := []interface{}{userID}
	for i, role := range roles {
		placeholders[i] = "?"
		args = append(args, role)
	}

	roleClause := ""
	if len(roles) > 0 {
		roleClause = "OR en.assigned_to_role IN (" + strings.Join(placeholders, ",") + ")"
	}

	query := `
		SELECT en.id, aci.id, ac.id, ac.id, ac.requester_id,
		       aci.role_name, ac.justification, en.node_name,
		       CASE WHEN en.assigned_to_user_id IS NOT NULL THEN 'USER' ELSE 'ROLE_QUEUE' END,
		       en.assigned_at
		FROM execution_nodes en
		JOIN executions e ON en.execution_id = e.id
		JOIN access_cart_items aci ON e.cart_item_id = aci.id
		JOIN access_carts ac ON aci.cart_id = ac.id
		WHERE en.status = 'PENDING'
		AND (en.assigned_to_user_id = ? ` + roleClause + `)`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.InboxItem
	for rows.Next() {
		var item models.InboxItem
		var assignedAtStr string
		if err := rows.Scan(&item.StepID, &item.CartItemID, &item.CartID, &item.CorrelationID,
			&item.RequesterID, &item.RoleRequested, &item.Justification,
			&item.StageName, &item.AssignmentType, &assignedAtStr); err != nil {
			return nil, err
		}
		item.AssignedAt, _ = parseTime(assignedAtStr)
		list = append(list, item)
	}
	return list, nil
}

// ========== AUDIT LOG ==========

func (r *SQLRepository) WriteAuditLog(log *models.AuditLog) error {
	_, err := r.db.Exec(`
		INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action, before_state, after_state, correlation_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.EntityType, log.EntityID, log.ActorUserID, log.Action, log.BeforeState, log.AfterState, log.CorrelationID)
	return err
}

func (r *SQLRepository) GetAuditTrail(instanceID string) ([]models.AuditLog, error) {
	rows, err := r.db.Query(`
		SELECT id, entity_type, entity_id, actor_user_id, action,
		       before_state, after_state, correlation_id, occurred_at
		FROM audit_logs
		WHERE (entity_id = ? OR correlation_id = (
		    SELECT correlation_id FROM audit_logs
		    WHERE entity_id = ? LIMIT 1
		))
		ORDER BY occurred_at ASC`, instanceID, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		var occurredAtStr string
		var actor sql.NullString
		if err := rows.Scan(&l.ID, &l.EntityType, &l.EntityID, &actor, &l.Action,
			&l.BeforeState, &l.AfterState, &l.CorrelationID, &occurredAtStr); err != nil {
			return nil, err
		}
		l.ActorUserID = actor.String
		l.OccurredAt, _ = parseTime(occurredAtStr)
		list = append(list, l)
	}
	return list, nil
}

// ========== SSE NOTIFICATIONS ==========

func (r *SQLRepository) CreateSSENotification(n *models.SSENotification) error {
	_, err := r.db.Exec(`
		INSERT INTO sse_notifications (id, user_id, event_type, payload)
		VALUES (?, ?, ?, ?)`,
		n.ID, n.UserID, n.EventType, n.Payload)
	return err
}

func (r *SQLRepository) GetUnreadSSENotifications(userID string) ([]models.SSENotification, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, event_type, payload, is_read, created_at
		FROM sse_notifications
		WHERE user_id = ? AND is_read = 0
		ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.SSENotification
	for rows.Next() {
		var n models.SSENotification
		var createdAtStr string
		if err := rows.Scan(&n.ID, &n.UserID, &n.EventType, &n.Payload, &n.IsRead, &createdAtStr); err != nil {
			return nil, err
		}
		n.CreatedAt, _ = parseTime(createdAtStr)
		list = append(list, n)
	}
	return list, nil
}

func (r *SQLRepository) MarkNotificationsRead(userID string) error {
	_, err := r.db.Exec(`
		UPDATE sse_notifications SET is_read = 1
		WHERE user_id = ? AND is_read = 0`, userID)
	return err
}

// ========== TRANSACTION HELPERS ==========

func (r *SQLRepository) checkCartCompletionTx(tx *sql.Tx, cartItemID string) error {
	var cartID string
	err := tx.QueryRow(`SELECT cart_id FROM access_cart_items WHERE id = ?`, cartItemID).Scan(&cartID)
	if err != nil {
		return err
	}

	rows, err := tx.Query(`SELECT status FROM access_cart_items WHERE cart_id = ?`, cartID)
	if err != nil {
		return err
	}
	defer rows.Close()

	allDone := true
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		if s != "APPROVED" && s != "REJECTED" && s != "CANCELLED" {
			allDone = false
			break
		}
	}

	if allDone {
		_, err = tx.Exec(`
			UPDATE access_carts SET status = 'COMPLETED', completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`, cartID)
		return err
	}
	return nil
}

// WithdrawCart cancels a submitted cart and all its pending executions and items.
func (r *SQLRepository) WithdrawCart(cartID string, actorUserID string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update access_carts status
	_, err = tx.Exec(`
		UPDATE access_carts 
		SET status = 'CANCELLED', completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ? AND status IN ('SUBMITTED', 'IN_PROGRESS')`, cartID)
	if err != nil {
		return err
	}

	// Update access_cart_items status
	_, err = tx.Exec(`
		UPDATE access_cart_items 
		SET status = 'CANCELLED', updated_at = CURRENT_TIMESTAMP 
		WHERE cart_id = ? AND status IN ('PENDING', 'IN_PROGRESS')`, cartID)
	if err != nil {
		return err
	}

	// Find all executions in-progress for this cart
	rows, err := tx.Query(`
		SELECT id FROM executions 
		WHERE cart_item_id IN (SELECT id FROM access_cart_items WHERE cart_id = ?) 
		  AND status = 'IN_PROGRESS'`, cartID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var execIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		execIDs = append(execIDs, id)
	}
	rows.Close() // Close early before executing updates within transaction

	for _, execID := range execIDs {
		// Update execution status
		_, err = tx.Exec(`
			UPDATE executions 
			SET status = 'CANCELLED', completed_at = CURRENT_TIMESTAMP, version = version + 1 
			WHERE id = ?`, execID)
		if err != nil {
			return err
		}

		// Cancel pending nodes
		_, err = tx.Exec(`
			UPDATE execution_nodes 
			SET status = 'CANCELLED', version = version + 1 
			WHERE execution_id = ? AND status = 'PENDING'`, execID)
		if err != nil {
			return err
		}

		// Write audit log for the execution instance
		_, err = tx.Exec(`
			INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action, before_state, after_state, occurred_at)
			VALUES (?, 'WORKFLOW_INSTANCE', ?, ?, 'WITHDRAWN', 'IN_PROGRESS', 'CANCELLED', CURRENT_TIMESTAMP)`,
			uuid.New().String(), execID, actorUserID)
		if err != nil {
			return err
		}
	}

	// Write audit log for the cart
	_, err = tx.Exec(`
		INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action, before_state, after_state, occurred_at)
		VALUES (?, 'CART', ?, ?, 'WITHDRAWN', 'IN_PROGRESS', 'CANCELLED', CURRENT_TIMESTAMP)`,
		uuid.New().String(), cartID, actorUserID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetPendingStepsForCart returns details of all pending steps for a given cart.
func (r *SQLRepository) GetPendingStepsForCart(cartID string) ([]models.PendingStepDetail, error) {
	rows, err := r.db.Query(`
		SELECT en.id, aci.role_name, ac.id, en.assigned_to_user_id, en.assigned_to_role
		FROM execution_nodes en
		JOIN executions e ON en.execution_id = e.id
		JOIN access_cart_items aci ON e.cart_item_id = aci.id
		JOIN access_carts ac ON aci.cart_id = ac.id
		WHERE ac.id = ? AND en.status = 'PENDING'`, cartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.PendingStepDetail
	for rows.Next() {
		var pd models.PendingStepDetail
		if err := rows.Scan(&pd.StepID, &pd.RoleName, &pd.CartID, &pd.AssignedToUserID, &pd.AssignedToRole); err != nil {
			return nil, err
		}
		list = append(list, pd)
	}
	return list, nil
}
