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
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	if idx := strings.Index(s, " m="); idx != -1 {
		s = s[:idx]
		for _, l := range layouts {
			if t, err := time.Parse(l, s); err == nil {
				return t, nil
			}
		}
	}
	return time.Time{}, errors.New("failed to parse time: " + s)
}

// ========== WORKFLOW DEFINITIONS ==========

func (r *SQLRepository) GetWorkflowDefinitionByRoleID(roleID string) (*models.WorkflowDefinition, error) {
	var def models.WorkflowDefinition
	var createdAtStr string
	var supersedesID sql.NullString

	err := r.db.QueryRow(`
		SELECT wd.id, wd.name, wd.definition_key, wd.version, wd.is_current, wd.status,
		       wd.supersedes_id, wd.created_by, wd.created_at
		FROM workflow_definitions wd
		JOIN role_workflow_mappings rwm ON rwm.workflow_definition_id = wd.id
		WHERE rwm.role_id = ? AND rwm.is_active = 1
		LIMIT 1`, roleID).
		Scan(&def.ID, &def.Name, &def.DefinitionKey, &def.Version, &def.IsCurrent,
			&def.Status, &supersedesID, &def.CreatedBy, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // no mapping → caller handles auto-approve
		}
		return nil, err
	}
	if supersedesID.Valid {
		def.SupersedesID = &supersedesID.String
	}
	def.CreatedAt, _ = parseTime(createdAtStr)

	stages, err := r.getStagesForDefinition(def.ID)
	if err != nil {
		return nil, err
	}
	def.Stages = stages
	return &def, nil
}

func (r *SQLRepository) GetWorkflowDefinitionByID(id string) (*models.WorkflowDefinition, error) {
	var def models.WorkflowDefinition
	var createdAtStr string
	var supersedesID sql.NullString

	err := r.db.QueryRow(`
		SELECT id, name, definition_key, version, is_current, status,
		       supersedes_id, created_by, created_at
		FROM workflow_definitions WHERE id = ?`, id).
		Scan(&def.ID, &def.Name, &def.DefinitionKey, &def.Version, &def.IsCurrent,
			&def.Status, &supersedesID, &def.CreatedBy, &createdAtStr)
	if err != nil {
		return nil, err
	}
	if supersedesID.Valid {
		def.SupersedesID = &supersedesID.String
	}
	def.CreatedAt, _ = parseTime(createdAtStr)

	stages, err := r.getStagesForDefinition(def.ID)
	if err != nil {
		return nil, err
	}
	def.Stages = stages
	return &def, nil
}

func (r *SQLRepository) getStagesForDefinition(defID string) ([]models.WorkflowStageDefinition, error) {
	rows, err := r.db.Query(`
		SELECT id, workflow_definition_id, sequence_order, name, type, approval_tree
		FROM workflow_stage_definitions
		WHERE workflow_definition_id = ?
		ORDER BY sequence_order ASC`, defID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stages []models.WorkflowStageDefinition
	for rows.Next() {
		var s models.WorkflowStageDefinition
		var treeStr sql.NullString
		if err := rows.Scan(&s.ID, &s.WorkflowDefinitionID, &s.SequenceOrder,
			&s.Name, &s.Type, &treeStr); err != nil {
			return nil, err
		}
		if treeStr.Valid && treeStr.String != "" {
			var tree models.ApprovalNode
			if err := json.Unmarshal([]byte(treeStr.String), &tree); err == nil {
				s.ApprovalTree = &tree
			}
		}
		stages = append(stages, s)
	}
	return stages, nil
}

func convertUpsertNode(req *models.UpsertNodeRequest, stageID string, counter *int) *models.ApprovalNode {
	if req == nil {
		return nil
	}
	*counter++
	node := &models.ApprovalNode{
		ID:             fmt.Sprintf("%s-node-%d", stageID, *counter),
		Type:           req.Type,
		Value:          req.Value,
		GroupCondition: req.GroupCondition,
	}
	for i := range req.Children {
		child := convertUpsertNode(&req.Children[i], stageID, counter)
		if child != nil {
			node.Children = append(node.Children, *child)
		}
	}
	return node
}

// UpsertWorkflowDefinition creates a new versioned definition and updates the role mapping atomically.
func (r *SQLRepository) UpsertWorkflowDefinition(roleID, definitionKey, newID, createdBy string,
	req models.UpsertWorkflowRequest) (*models.WorkflowDefinition, error) {

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Find current version number and existing mapping
	var oldDefID sql.NullString
	var oldVersion int
	_ = tx.QueryRow(`
		SELECT wd.id, wd.version
		FROM workflow_definitions wd
		JOIN role_workflow_mappings rwm ON rwm.workflow_definition_id = wd.id
		WHERE rwm.role_id = ? AND rwm.is_active = 1
		LIMIT 1`, roleID).Scan(&oldDefID, &oldVersion)

	newVersion := oldVersion + 1

	// Mark old definition superseded
	var supersedesVal interface{}
	if oldDefID.Valid {
		supersedesVal = oldDefID.String
		if _, err := tx.Exec(`
			UPDATE workflow_definitions SET is_current = 0, status = 'SUPERSEDED'
			WHERE id = ?`, oldDefID.String); err != nil {
			return nil, err
		}
	}

	// Insert new definition
	if _, err := tx.Exec(`
		INSERT INTO workflow_definitions (id, name, definition_key, version, is_current, status, supersedes_id, created_by)
		VALUES (?, ?, ?, ?, 1, 'ACTIVE', ?, ?)`,
		newID, req.Name, definitionKey, newVersion, supersedesVal, createdBy); err != nil {
		return nil, err
	}

	// Insert stages
	for _, stReq := range req.Stages {
		stageID := newID + "-stage-" + strings.ReplaceAll(stReq.Name, " ", "_")
		var treeJSON []byte
		if stReq.ApprovalTree != nil {
			counter := 0
			treeNode := convertUpsertNode(stReq.ApprovalTree, stageID, &counter)
			var err error
			treeJSON, err = json.Marshal(treeNode)
			if err != nil {
				return nil, err
			}
		}

		if _, err := tx.Exec(`
			INSERT INTO workflow_stage_definitions
			(id, workflow_definition_id, sequence_order, name, type, approval_tree)
			VALUES (?, ?, ?, ?, ?, ?)`,
			stageID, newID, stReq.SequenceOrder, stReq.Name, string(stReq.Type), string(treeJSON)); err != nil {
			return nil, err
		}
	}

	// Upsert role mapping
	mappingID := "rwm-" + roleID
	if _, err := tx.Exec(`
		INSERT INTO role_workflow_mappings (id, role_id, workflow_definition_id, is_active, created_by)
		VALUES (?, ?, ?, 1, ?)
		ON CONFLICT(role_id) DO UPDATE SET
			workflow_definition_id = excluded.workflow_definition_id,
			is_active = 1,
			effective_from = CURRENT_TIMESTAMP,
			effective_to = NULL`,
		mappingID, roleID, newID, createdBy); err != nil {
			return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetWorkflowDefinitionByID(newID)
}

func (r *SQLRepository) ListWorkflowDefinitions() ([]models.WorkflowDefinition, error) {
	rows, err := r.db.Query(`
		SELECT id, name, definition_key, version, is_current, status, created_by, created_at
		FROM workflow_definitions ORDER BY definition_key, version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.WorkflowDefinition
	for rows.Next() {
		var d models.WorkflowDefinition
		var createdAtStr string
		if err := rows.Scan(&d.ID, &d.Name, &d.DefinitionKey, &d.Version, &d.IsCurrent,
			&d.Status, &d.CreatedBy, &createdAtStr); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = parseTime(createdAtStr)
		list = append(list, d)
	}
	return list, nil
}

func (r *SQLRepository) GetDefinitionHistory(definitionKey string) ([]models.WorkflowDefinition, error) {
	rows, err := r.db.Query(`
		SELECT id, name, definition_key, version, is_current, status, supersedes_id, created_by, created_at
		FROM workflow_definitions WHERE definition_key = ?
		ORDER BY version DESC`, definitionKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.WorkflowDefinition
	for rows.Next() {
		var d models.WorkflowDefinition
		var createdAtStr string
		var sup sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &d.DefinitionKey, &d.Version, &d.IsCurrent,
			&d.Status, &sup, &d.CreatedBy, &createdAtStr); err != nil {
			return nil, err
		}
		if sup.Valid {
			d.SupersedesID = &sup.String
		}
		d.CreatedAt, _ = parseTime(createdAtStr)
		list = append(list, d)
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
	return &c, nil
}

func (r *SQLRepository) ListCarts(requesterID string) ([]models.AccessCart, error) {
	rows, err := r.db.Query(`
		SELECT id, requester_id, status, justification, submitted_at, completed_at,
		       version, created_at, updated_at
		FROM access_carts WHERE requester_id = ?
		ORDER BY created_at DESC`, requesterID)
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
		list = append(list, c)
	}
	return list, nil
}

func (r *SQLRepository) UpdateCartStatus(cartID, status string) error {
	_, err := r.db.Exec(`
		UPDATE access_carts SET status = ?, updated_at = CURRENT_TIMESTAMP,
		submitted_at = CASE WHEN ? = 'SUBMITTED' THEN CURRENT_TIMESTAMP ELSE submitted_at END,
		completed_at = CASE WHEN ? IN ('COMPLETED', 'CANCELLED') THEN CURRENT_TIMESTAMP ELSE completed_at END
		WHERE id = ?`, status, status, status, cartID)
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
		SELECT id, cart_id, role_id, role_name, status, workflow_instance_id, created_at, updated_at
		FROM access_cart_items WHERE cart_id = ?`, cartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.CartItem
	for rows.Next() {
		var item models.CartItem
		var createdAtStr, updatedAtStr string
		var wfInstID sql.NullString
		if err := rows.Scan(&item.ID, &item.CartID, &item.RoleID, &item.RoleName,
			&item.Status, &wfInstID, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		if wfInstID.Valid {
			item.WorkflowInstanceID = &wfInstID.String
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
	var wfInstID sql.NullString
	err := r.db.QueryRow(`
		SELECT id, cart_id, role_id, role_name, status, workflow_instance_id, created_at, updated_at
		FROM access_cart_items WHERE id = ?`, itemID).
		Scan(&item.ID, &item.CartID, &item.RoleID, &item.RoleName,
			&item.Status, &wfInstID, &createdAtStr, &updatedAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("cart item not found")
		}
		return nil, err
	}
	if wfInstID.Valid {
		item.WorkflowInstanceID = &wfInstID.String
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

func (r *SQLRepository) UpdateCartItemStatus(itemID, status string, wfInstanceID *string) error {
	_, err := r.db.Exec(`
		UPDATE access_cart_items SET status = ?, workflow_instance_id = COALESCE(?, workflow_instance_id),
		updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, wfInstanceID, itemID)
	return err
}

// CheckAllCartItemsTerminal returns true if every item in the cart is in a terminal state.
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
			overallStatus = "COMPLETED" // cart is completed even with rejections
		}
	}
	return allDone, overallStatus, nil
}

// ========== WORKFLOW INSTANCES ==========

func (r *SQLRepository) CreateWorkflowInstance(inst *models.WorkflowInstance) error {
	_, err := r.db.Exec(`
		INSERT INTO workflow_instances (id, workflow_definition_id, cart_item_id, status, current_stage_seq)
		VALUES (?, ?, ?, 'IN_PROGRESS', ?)`,
		inst.ID, inst.WorkflowDefinitionID, inst.CartItemID, inst.CurrentStageSeq)
	return err
}

func (r *SQLRepository) GetWorkflowInstance(instanceID string) (*models.WorkflowInstance, error) {
	var inst models.WorkflowInstance
	var startedAtStr string
	var completedAt sql.NullString
	err := r.db.QueryRow(`
		SELECT id, workflow_definition_id, cart_item_id, status, current_stage_seq,
		       started_at, completed_at, version
		FROM workflow_instances WHERE id = ?`, instanceID).
		Scan(&inst.ID, &inst.WorkflowDefinitionID, &inst.CartItemID, &inst.Status,
			&inst.CurrentStageSeq, &startedAtStr, &completedAt, &inst.Version)
	if err != nil {
		return nil, err
	}
	inst.StartedAt, _ = parseTime(startedAtStr)
	if completedAt.Valid {
		t, _ := parseTime(completedAt.String)
		inst.CompletedAt = &t
	}
	return &inst, nil
}

func (r *SQLRepository) UpdateWorkflowInstanceStatus(instanceID, status string, nextStageSeq int) error {
	_, err := r.db.Exec(`
		UPDATE workflow_instances SET status = ?, current_stage_seq = ?,
		completed_at = CASE WHEN ? IN ('COMPLETED','REJECTED','CANCELLED') THEN CURRENT_TIMESTAMP ELSE completed_at END,
		version = version + 1
		WHERE id = ?`, status, nextStageSeq, status, instanceID)
	return err
}

// ========== WORKFLOW STEPS ==========

func (r *SQLRepository) CreateWorkflowStep(step *models.WorkflowStep) error {
	_, err := r.db.Exec(`
		INSERT INTO workflow_steps
		(id, workflow_instance_id, stage_definition_id, node_id, assigned_to_user_id, assigned_to_role, status)
		VALUES (?, ?, ?, ?, ?, ?, 'PENDING')`,
		step.ID, step.WorkflowInstanceID, step.StageDefinitionID, step.NodeID,
		step.AssignedToUserID, step.AssignedToRole)
	return err
}

func (r *SQLRepository) GetWorkflowStep(stepID string) (*models.WorkflowStep, error) {
	var step models.WorkflowStep
	var assignedAtStr string
	var actedAt sql.NullString
	var assignedToUser, assignedToRole, ackedBy sql.NullString
	err := r.db.QueryRow(`
		SELECT id, workflow_instance_id, stage_definition_id, node_id,
		       assigned_to_user_id, assigned_to_role, status, decision_comment,
		       acted_by_user_id, assigned_at, acted_at, version
		FROM workflow_steps WHERE id = ?`, stepID).
		Scan(&step.ID, &step.WorkflowInstanceID, &step.StageDefinitionID, &step.NodeID,
			&assignedToUser, &assignedToRole, &step.Status, &step.DecisionComment,
			&ackedBy, &assignedAtStr, &actedAt, &step.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("step not found")
		}
		return nil, err
	}
	if assignedToUser.Valid {
		step.AssignedToUserID = &assignedToUser.String
	}
	if assignedToRole.Valid {
		step.AssignedToRole = &assignedToRole.String
	}
	if ackedBy.Valid {
		step.ActedByUserID = &ackedBy.String
	}
	step.AssignedAt, _ = parseTime(assignedAtStr)
	if actedAt.Valid {
		t, _ := parseTime(actedAt.String)
		step.ActedAt = &t
	}
	return &step, nil
}

func (r *SQLRepository) ActionWorkflowStep(stepID, status, comment, actorUserID string) error {
	_, err := r.db.Exec(`
		UPDATE workflow_steps SET status = ?, decision_comment = ?, acted_by_user_id = ?,
		acted_at = CURRENT_TIMESTAMP, version = version + 1
		WHERE id = ? AND status = 'PENDING'`, status, comment, actorUserID, stepID)
	return err
}

// GetStepsForInstanceStage returns all steps for a given instance at a given stage sequence order.
func (r *SQLRepository) GetStepsForInstanceStage(instanceID string, stageSeq int) ([]models.WorkflowStep, error) {
	rows, err := r.db.Query(`
		SELECT ws.id, ws.workflow_instance_id, ws.stage_definition_id, ws.node_id,
		       ws.assigned_to_user_id, ws.assigned_to_role, ws.status, ws.decision_comment,
		       ws.acted_by_user_id, ws.assigned_at, ws.acted_at, ws.version
		FROM workflow_steps ws
		JOIN workflow_stage_definitions wsd ON ws.stage_definition_id = wsd.id
		WHERE ws.workflow_instance_id = ? AND wsd.sequence_order = ?`, instanceID, stageSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.WorkflowStep
	for rows.Next() {
		var step models.WorkflowStep
		var assignedAtStr string
		var actedAt sql.NullString
		var u, ro, ab sql.NullString
		if err := rows.Scan(&step.ID, &step.WorkflowInstanceID, &step.StageDefinitionID, &step.NodeID,
			&u, &ro, &step.Status, &step.DecisionComment, &ab,
			&assignedAtStr, &actedAt, &step.Version); err != nil {
			return nil, err
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
		list = append(list, step)
	}
	return list, nil
}

// GetInboxItems returns pending steps assigned to a user or one of their roles.
func (r *SQLRepository) GetInboxItems(userID string, roles []string) ([]models.InboxItem, error) {
	placeholders := make([]string, len(roles))
	args := []interface{}{userID}
	for i, role := range roles {
		placeholders[i] = "?"
		args = append(args, role)
	}

	roleClause := ""
	if len(roles) > 0 {
		roleClause = "OR ws.assigned_to_role IN (" + strings.Join(placeholders, ",") + ")"
	}

	query := `
		SELECT ws.id, aci.id, ac.id, ac.id, ac.requester_id,
		       aci.role_name, ac.justification, wsd.name,
		       CASE WHEN ws.assigned_to_user_id IS NOT NULL THEN 'USER' ELSE 'ROLE_QUEUE' END,
		       ws.assigned_at
		FROM workflow_steps ws
		JOIN workflow_instances wi ON ws.workflow_instance_id = wi.id
		JOIN access_cart_items aci ON wi.cart_item_id = aci.id
		JOIN access_carts ac ON aci.cart_id = ac.id
		JOIN workflow_stage_definitions wsd ON ws.stage_definition_id = wsd.id
		WHERE ws.status = 'PENDING'
		AND (ws.assigned_to_user_id = ? ` + roleClause + `)`

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
		INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action,
		                        before_state, after_state, correlation_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.EntityType, log.EntityID, log.ActorUserID, log.Action,
		log.BeforeState, log.AfterState, log.CorrelationID)
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
	payload, _ := json.Marshal(n.Payload)
	_, err := r.db.Exec(`
		INSERT INTO sse_notifications (id, user_id, event_type, payload)
		VALUES (?, ?, ?, ?)`, n.ID, n.UserID, n.EventType, string(payload))
	return err
}

func (r *SQLRepository) GetUnreadSSENotifications(userID string) ([]models.SSENotification, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, event_type, payload, is_read, created_at
		FROM sse_notifications WHERE user_id = ? AND is_read = 0
		ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.SSENotification
	for rows.Next() {
		var n models.SSENotification
		var createdAtStr string
		var isRead int
		if err := rows.Scan(&n.ID, &n.UserID, &n.EventType, &n.Payload, &isRead, &createdAtStr); err != nil {
			return nil, err
		}
		n.IsRead = isRead == 1
		n.CreatedAt, _ = parseTime(createdAtStr)
		list = append(list, n)
	}
	return list, nil
}

func (r *SQLRepository) MarkNotificationsRead(userID string) error {
	_, err := r.db.Exec(`UPDATE sse_notifications SET is_read = 1 WHERE user_id = ?`, userID)
	return err
}

type NodeStatus string

const (
	NodeStatusPending  NodeStatus = "PENDING"
	NodeStatusApproved NodeStatus = "APPROVED"
	NodeStatusRejected NodeStatus = "REJECTED"
)

func (r *SQLRepository) ActionStepAndProgress(stepID, actionStatus, comment, actorUserID string) (
	inst *models.WorkflowInstance,
	nextSteps []models.WorkflowStep,
	stageStatus NodeStatus,
	message string,
	err error,
) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, nil, "", "", err
	}
	defer tx.Rollback()

	// 1. Update the step status
	res, err := tx.Exec(`
		UPDATE workflow_steps SET status = ?, decision_comment = ?, acted_by_user_id = ?,
		acted_at = CURRENT_TIMESTAMP, version = version + 1
		WHERE id = ? AND status = 'PENDING'`, actionStatus, comment, actorUserID, stepID)
	if err != nil {
		return nil, nil, "", "", err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return nil, nil, "", "", errors.New("step is no longer pending")
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

	// 2. Load step details
	var instanceID, stageDefID, nodeID string
	err = tx.QueryRow(`
		SELECT workflow_instance_id, stage_definition_id, node_id
		FROM workflow_steps WHERE id = ?`, stepID).Scan(&instanceID, &stageDefID, &nodeID)
	if err != nil {
		return nil, nil, "", "", err
	}

	// 3. Load workflow instance
	inst = &models.WorkflowInstance{}
	var startedAtStr string
	var completedAt sql.NullString
	err = tx.QueryRow(`
		SELECT id, workflow_definition_id, cart_item_id, status, current_stage_seq, started_at, completed_at, version
		FROM workflow_instances WHERE id = ?`, instanceID).
		Scan(&inst.ID, &inst.WorkflowDefinitionID, &inst.CartItemID, &inst.Status,
			&inst.CurrentStageSeq, &startedAtStr, &completedAt, &inst.Version)
	if err != nil {
		return nil, nil, "", "", err
	}
	inst.StartedAt, _ = parseTime(startedAtStr)
	if completedAt.Valid {
		t, _ := parseTime(completedAt.String)
		inst.CompletedAt = &t
	}

	// 4. Load all stages for this definition (read-only, can be done via tx)
	stages, err := r.getStagesForDefinitionTx(tx, inst.WorkflowDefinitionID)
	if err != nil {
		return nil, nil, "", "", err
	}

	var currentStage *models.WorkflowStageDefinition
	for i, st := range stages {
		if st.SequenceOrder == inst.CurrentStageSeq {
			currentStage = &stages[i]
			break
		}
	}
	if currentStage == nil {
		return nil, nil, "", "", errors.New("current stage not found in definition")
	}

	// 5. Load steps for current stage (using tx)
	rows, err := tx.Query(`
		SELECT ws.id, ws.workflow_instance_id, ws.stage_definition_id, ws.node_id,
		       ws.assigned_to_user_id, ws.assigned_to_role, ws.status, ws.decision_comment,
		       ws.acted_by_user_id, ws.assigned_at, ws.acted_at, ws.version
		FROM workflow_steps ws
		JOIN workflow_stage_definitions wsd ON ws.stage_definition_id = wsd.id
		WHERE ws.workflow_instance_id = ? AND wsd.sequence_order = ?`, inst.ID, inst.CurrentStageSeq)
	if err != nil {
		return nil, nil, "", "", err
	}
	defer rows.Close()

	var stageSteps []models.WorkflowStep
	for rows.Next() {
		var step models.WorkflowStep
		var assignedAtStr string
		var actedAt sql.NullString
		var u, ro, ab sql.NullString
		if err := rows.Scan(&step.ID, &step.WorkflowInstanceID, &step.StageDefinitionID, &step.NodeID,
			&u, &ro, &step.Status, &step.DecisionComment, &ab,
			&assignedAtStr, &actedAt, &step.Version); err != nil {
			return nil, nil, "", "", err
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
		stageSteps = append(stageSteps, step)
	}

	// 6. Construct steps map and evaluate
	stepsMap := make(map[string][]models.WorkflowStep)
	for _, s2 := range stageSteps {
		stepsMap[s2.NodeID] = append(stepsMap[s2.NodeID], s2)
	}

	stageStatus = evaluateStageTree(currentStage.ApprovalTree, stepsMap)

	if stageStatus == NodeStatusRejected {
		// Update workflow instance to REJECTED
		_, err = tx.Exec(`
			UPDATE workflow_instances SET status = 'REJECTED',
			completed_at = CURRENT_TIMESTAMP, version = version + 1
			WHERE id = ?`, inst.ID)
		if err != nil {
			return nil, nil, "", "", err
		}
		inst.Status = "REJECTED"

		// Update cart item to REJECTED
		_, err = tx.Exec(`
			UPDATE access_cart_items SET status = 'REJECTED', updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`, inst.CartItemID)
		if err != nil {
			return nil, nil, "", "", err
		}

		// Check and update cart completion
		if err := r.checkCartCompletionTx(tx, inst.CartItemID); err != nil {
			return nil, nil, "", "", err
		}

		// Write audit log
		_, err = tx.Exec(`
			INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action)
			VALUES (?, 'WORKFLOW_INSTANCE', ?, ?, 'REJECTED')`,
			uuid.New().String(), inst.ID, actorUserID)
		if err != nil {
			return nil, nil, "", "", err
		}

		if err := tx.Commit(); err != nil {
			return nil, nil, "", "", err
		}
		return inst, nil, stageStatus, "Request rejected", nil
	}

	if stageStatus == NodeStatusPending {
		if err := tx.Commit(); err != nil {
			return nil, nil, "", "", err
		}
		return inst, nil, stageStatus, "Waiting for other approvals", nil
	}

	// stageStatus == NodeStatusApproved -> Advance to next stage or complete
	nextSeq := -1
	for _, st := range stages {
		if st.SequenceOrder > inst.CurrentStageSeq {
			if nextSeq == -1 || st.SequenceOrder < nextSeq {
				nextSeq = st.SequenceOrder
			}
		}
	}

	if nextSeq == -1 {
		// No more stages — COMPLETED
		_, err = tx.Exec(`
			UPDATE workflow_instances SET status = 'COMPLETED',
			completed_at = CURRENT_TIMESTAMP, version = version + 1
			WHERE id = ?`, inst.ID)
		if err != nil {
			return nil, nil, "", "", err
		}
		inst.Status = "COMPLETED"

		_, err = tx.Exec(`
			UPDATE access_cart_items SET status = 'APPROVED', updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`, inst.CartItemID)
		if err != nil {
			return nil, nil, "", "", err
		}

		// Check and update cart completion
		if err := r.checkCartCompletionTx(tx, inst.CartItemID); err != nil {
			return nil, nil, "", "", err
		}

		// Write audit log
		_, err = tx.Exec(`
			INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action)
			VALUES (?, 'WORKFLOW_INSTANCE', ?, ?, 'COMPLETED')`,
			uuid.New().String(), inst.ID, actorUserID)
		if err != nil {
			return nil, nil, "", "", err
		}

		if err := tx.Commit(); err != nil {
			return nil, nil, "", "", err
		}
		return inst, nil, stageStatus, "All stages approved — access granted", nil
	}

	// Advance to next stage
	_, err = tx.Exec(`
		UPDATE workflow_instances SET current_stage_seq = ?, version = version + 1
		WHERE id = ?`, nextSeq, inst.ID)
	if err != nil {
		return nil, nil, "", "", err
	}
	inst.CurrentStageSeq = nextSeq

	var nextStage *models.WorkflowStageDefinition
	for i, st := range stages {
		if st.SequenceOrder == nextSeq {
			nextStage = &stages[i]
			break
		}
	}
	if nextStage == nil {
		return nil, nil, "", "", errors.New("next stage not found in definition")
	}

	correlationID := uuid.New().String()
	nextLeaves := GetLeafNodes(nextStage.ApprovalTree)
	for _, leaf := range nextLeaves {
		ns := models.WorkflowStep{
			ID:                 uuid.New().String(),
			WorkflowInstanceID: inst.ID,
			StageDefinitionID:  nextStage.ID,
			NodeID:             leaf.ID,
			Status:             "PENDING",
			AssignedAt:         time.Now(),
		}
		switch leaf.Type {
		case "USER":
			val := leaf.Value
			ns.AssignedToUserID = &val
		case "ROLE":
			val := leaf.Value
			ns.AssignedToRole = &val
		}

		_, err = tx.Exec(`
			INSERT INTO workflow_steps
			(id, workflow_instance_id, stage_definition_id, node_id, assigned_to_user_id, assigned_to_role, status)
			VALUES (?, ?, ?, ?, ?, ?, 'PENDING')`,
			ns.ID, ns.WorkflowInstanceID, ns.StageDefinitionID, ns.NodeID,
			ns.AssignedToUserID, ns.AssignedToRole)
		if err != nil {
			return nil, nil, "", "", err
		}

		_, err = tx.Exec(`
			INSERT INTO audit_logs (id, entity_type, entity_id, actor_user_id, action, after_state, correlation_id)
			VALUES (?, 'WORKFLOW_STEP', ?, 'system', 'ASSIGNED', ?, ?)`,
			uuid.New().String(), ns.ID, fmt.Sprintf(`{"stage":"%s","seq":%d,"node_id":"%s"}`, nextStage.Name, nextSeq, leaf.ID), correlationID)
		if err != nil {
			return nil, nil, "", "", err
		}

		nextSteps = append(nextSteps, ns)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, "", "", err
	}

	return inst, nextSteps, stageStatus, fmt.Sprintf("Advanced to stage %d", nextSeq), nil
}

func (r *SQLRepository) DelegateStep(stepID, comment, actorUserID, delegateUserID string, newStepID string) (*models.WorkflowStep, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Update original step
	res, err := tx.Exec(`
		UPDATE workflow_steps SET status = 'DELEGATED', decision_comment = ?, acted_by_user_id = ?,
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
	var instID, stageDefID, nodeID string
	err = tx.QueryRow(`
		SELECT workflow_instance_id, stage_definition_id, node_id
		FROM workflow_steps WHERE id = ?`, stepID).Scan(&instID, &stageDefID, &nodeID)
	if err != nil {
		return nil, err
	}

	// Create new step
	newStep := &models.WorkflowStep{
		ID:                 newStepID,
		WorkflowInstanceID: instID,
		StageDefinitionID:  stageDefID,
		NodeID:             nodeID,
		AssignedToUserID:   &delegateUserID,
		Status:             "PENDING",
		AssignedAt:         time.Now(),
	}

	_, err = tx.Exec(`
		INSERT INTO workflow_steps
		(id, workflow_instance_id, stage_definition_id, node_id, assigned_to_user_id, status)
		VALUES (?, ?, ?, ?, ?, 'PENDING')`,
		newStep.ID, newStep.WorkflowInstanceID, newStep.StageDefinitionID, newStep.NodeID, newStep.AssignedToUserID)
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

func (r *SQLRepository) getStagesForDefinitionTx(tx *sql.Tx, defID string) ([]models.WorkflowStageDefinition, error) {
	rows, err := tx.Query(`
		SELECT id, workflow_definition_id, sequence_order, name, type, approval_tree
		FROM workflow_stage_definitions
		WHERE workflow_definition_id = ?
		ORDER BY sequence_order ASC`, defID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stages []models.WorkflowStageDefinition
	for rows.Next() {
		var s models.WorkflowStageDefinition
		var treeStr sql.NullString
		if err := rows.Scan(&s.ID, &s.WorkflowDefinitionID, &s.SequenceOrder,
			&s.Name, &s.Type, &treeStr); err != nil {
			return nil, err
		}
		if treeStr.Valid && treeStr.String != "" {
			var tree models.ApprovalNode
			if err := json.Unmarshal([]byte(treeStr.String), &tree); err == nil {
				s.ApprovalTree = &tree
			}
		}
		stages = append(stages, s)
	}
	return stages, nil
}

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

func evaluateStageTree(node *models.ApprovalNode, stepsMap map[string][]models.WorkflowStep) NodeStatus {
	if node == nil {
		return NodeStatusApproved
	}

	if node.Type == "USER" || node.Type == "ROLE" {
		steps := stepsMap[node.ID]
		if len(steps) == 0 {
			return NodeStatusPending
		}

		var activeStep *models.WorkflowStep
		for _, step := range steps {
			if step.Status != "DELEGATED" {
				if activeStep == nil || step.AssignedAt.After(activeStep.AssignedAt) {
					activeStep = &step
				}
			}
		}

		if activeStep == nil {
			return NodeStatusPending
		}

		switch activeStep.Status {
		case "APPROVED":
			return NodeStatusApproved
		case "REJECTED":
			return NodeStatusRejected
		default:
			return NodeStatusPending
		}
	}

	if node.Type == "GROUP" {
		if len(node.Children) == 0 {
			return NodeStatusApproved
		}

		if node.GroupCondition == models.ConditionOr {
			allRejected := true
			for i := range node.Children {
				status := evaluateStageTree(&node.Children[i], stepsMap)
				if status == NodeStatusApproved {
					return NodeStatusApproved
				}
				if status != NodeStatusRejected {
					allRejected = false
				}
			}
			if allRejected {
				return NodeStatusRejected
			}
			return NodeStatusPending
		}

		if node.GroupCondition == models.ConditionAnd {
			allApproved := true
			for i := range node.Children {
				status := evaluateStageTree(&node.Children[i], stepsMap)
				if status == NodeStatusRejected {
					return NodeStatusRejected
				}
				if status != NodeStatusApproved {
					allApproved = false
				}
			}
			if allApproved {
				return NodeStatusApproved
			}
			return NodeStatusPending
		}
	}

	return NodeStatusPending
}

func GetLeafNodes(node *models.ApprovalNode) []*models.ApprovalNode {
	if node == nil {
		return nil
	}
	if node.Type == "USER" || node.Type == "ROLE" {
		return []*models.ApprovalNode{node}
	}
	var leaves []*models.ApprovalNode
	for i := range node.Children {
		leaves = append(leaves, GetLeafNodes(&node.Children[i])...)
	}
	return leaves
}
