package repository

import (
	"database/sql"
	"embed"
	"errors"
	"icarus-admin-ms/internal/models"
	"strings"
	"time"

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

// InitDB initializes SQLite and runs up migrations sequentially.
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	_, _ = db.Exec("PRAGMA foreign_keys = ON;")

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
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

		statements := strings.Split(string(upSQL), ";")
		for _, stmt := range statements {
			trimmed := strings.TrimSpace(stmt)
			if trimmed == "" {
				continue
			}
			if _, err := db.Exec(trimmed); err != nil {
				db.Close()
				return nil, err
			}
		}

		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			db.Close()
			return nil, err
		}
	}

	return db, nil
}

func parseTime(timeStr string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05.999999999 -0700 MST",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, timeStr); err == nil {
			return t, nil
		}
	}
	// Fallback: try parsing with space suffix trimmed
	if idx := strings.Index(timeStr, " m="); idx != -1 {
		timeStr = timeStr[:idx]
		for _, layout := range layouts {
			if t, err := time.Parse(layout, timeStr); err == nil {
				return t, nil
			}
		}
	}
	return time.Time{}, errors.New("failed to parse time: " + timeStr)
}

// === MODULES & PERMISSIONS QUERY FUNCTIONS ===

func (r *SQLRepository) GetModuleByCode(code string) (*models.Module, error) {
	var m models.Module
	var createdAtStr string
	err := r.db.QueryRow(`SELECT id, code, name, base_url, is_active, created_at FROM modules WHERE code = ?`, code).
		Scan(&m.ID, &m.Code, &m.Name, &m.BaseURL, &m.IsActive, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("module not found")
		}
		return nil, err
	}
	m.CreatedAt, _ = parseTime(createdAtStr)
	return &m, nil
}

func (r *SQLRepository) GetPermissionsByModuleID(moduleID string) ([]models.Permission, error) {
	rows, err := r.db.Query(`SELECT id, module_id, action, path_pattern, method, description, created_at FROM permissions WHERE module_id = ? ORDER BY path_pattern ASC`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []models.Permission
	for rows.Next() {
		var p models.Permission
		var createdAtStr string
		var pathPattern, mthd, desc sql.NullString
		if err := rows.Scan(&p.ID, &p.ModuleID, &p.Action, &pathPattern, &mthd, &desc, &createdAtStr); err != nil {
			return nil, err
		}
		p.PathPattern = pathPattern.String
		p.Method = mthd.String
		p.Description = desc.String
		p.CreatedAt, _ = parseTime(createdAtStr)
		permissions = append(permissions, p)
	}
	return permissions, nil
}

// === NEW MODULES & TENANTS REGISTRY ===

func (r *SQLRepository) RegisterModule(module *models.Module, permissions []models.Permission) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Upsert module
	_, err = tx.Exec(`
		INSERT INTO modules (id, code, name, base_url, is_active) 
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, base_url = excluded.base_url, is_active = excluded.is_active`,
		module.ID, module.Code, module.Name, module.BaseURL, module.IsActive)
	if err != nil {
		return err
	}

	// Delete existing permissions for this module
	_, err = tx.Exec(`DELETE FROM permissions WHERE module_id = ?`, module.ID)
	if err != nil {
		return err
	}

	// Re-insert permissions
	for _, p := range permissions {
		_, err = tx.Exec(`
			INSERT INTO permissions (id, module_id, action, path_pattern, method, description) 
			VALUES (?, ?, ?, ?, ?, ?)`,
			p.ID, p.ModuleID, p.Action, p.PathPattern, p.Method, p.Description)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *SQLRepository) GetPermissionForRoute(moduleCode, path, method string) (*models.Permission, error) {
	var p models.Permission
	var pathPattern, mthd sql.NullString
	// Find the matching path pattern using matches logic or direct SQL LIKE
	// For simplicity, we can load all permissions for the module and match them in memory or do a pattern query.
	rows, err := r.db.Query(`
		SELECT p.id, p.module_id, p.action, p.path_pattern, p.method, p.description 
		FROM permissions p
		JOIN modules m ON p.module_id = m.id
		WHERE m.code = ?`, moduleCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&p.ID, &p.ModuleID, &p.Action, &pathPattern, &mthd, &p.Description)
		if err != nil {
			return nil, err
		}
		p.PathPattern = pathPattern.String
		p.Method = mthd.String

		// Match method and path
		methodMatch := p.Method == "*" || strings.ToUpper(p.Method) == "ANY" || strings.ToUpper(p.Method) == strings.ToUpper(method)
		if methodMatch && matchesPattern(path, p.PathPattern) {
			return &p, nil
		}
	}

	return nil, errors.New("no matching permission/route found")
}

// helper copied from gateway to resolve pattern matching in DB lookup
func matchesPattern(path, pattern string) bool {
	if pattern == "*" || pattern == "/*" {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(path, prefix)
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	return path == pattern
}

// ListTenants lists all tenants.
func (r *SQLRepository) ListTenants() ([]models.Tenant, error) {
	rows, err := r.db.Query("SELECT id, code, name, status, created_at FROM tenants ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Tenant
	for rows.Next() {
		var t models.Tenant
		var createdAtStr string
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Status, &createdAtStr); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = parseTime(createdAtStr)
		list = append(list, t)
	}
	return list, nil
}

// CreateTenant creates a new tenant.
func (r *SQLRepository) CreateTenant(t *models.Tenant) error {
	_, err := r.db.Exec(`
		INSERT INTO tenants (id, code, name, status) 
		VALUES (?, ?, ?, ?)`,
		t.ID, t.Code, t.Name, t.Status)
	return err
}

// UpdateTenant updates a tenant.
func (r *SQLRepository) UpdateTenant(t *models.Tenant) error {
	_, err := r.db.Exec(`
		UPDATE tenants SET name = ?, status = ? WHERE id = ?`,
		t.Name, t.Status, t.ID)
	return err
}

// DeleteTenant deletes a tenant.
func (r *SQLRepository) DeleteTenant(id string) error {
	_, err := r.db.Exec("DELETE FROM tenants WHERE id = ?", id)
	return err
}

// ListModules lists all modules.
func (r *SQLRepository) ListModules() ([]models.Module, error) {
	rows, err := r.db.Query("SELECT id, code, name, base_url, is_active, created_at FROM modules ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Module
	for rows.Next() {
		var m models.Module
		var createdAtStr string
		if err := rows.Scan(&m.ID, &m.Code, &m.Name, &m.BaseURL, &m.IsActive, &createdAtStr); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = parseTime(createdAtStr)
		list = append(list, m)
	}
	return list, nil
}

// UpdateModule updates module metadata.
func (r *SQLRepository) UpdateModule(m *models.Module) error {
	_, err := r.db.Exec(`
		UPDATE modules SET name = ?, base_url = ?, is_active = ? WHERE id = ?`,
		m.Name, m.BaseURL, m.IsActive, m.ID)
	return err
}

// DeleteModule deletes a module.
func (r *SQLRepository) DeleteModule(id string) error {
	_, err := r.db.Exec("DELETE FROM modules WHERE id = ?", id)
	return err
}

// ListApplications lists all applications.
func (r *SQLRepository) ListApplications() ([]models.Application, error) {
	rows, err := r.db.Query("SELECT id, code, name, description, created_at FROM applications ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Application
	for rows.Next() {
		var app models.Application
		var desc sql.NullString
		var createdAtStr string
		if err := rows.Scan(&app.ID, &app.Code, &app.Name, &desc, &createdAtStr); err != nil {
			return nil, err
		}
		app.Description = desc.String
		app.CreatedAt, _ = parseTime(createdAtStr)
		list = append(list, app)
	}
	return list, nil
}

// CreateApplication creates a new application.
func (r *SQLRepository) CreateApplication(app *models.Application) error {
	_, err := r.db.Exec(`
		INSERT INTO applications (id, code, name, description) 
		VALUES (?, ?, ?, ?)`,
		app.ID, app.Code, app.Name, app.Description)
	return err
}

// UpdateApplication updates an application.
func (r *SQLRepository) UpdateApplication(app *models.Application) error {
	_, err := r.db.Exec(`
		UPDATE applications SET name = ?, description = ? WHERE id = ?`,
		app.Name, app.Description, app.ID)
	return err
}

// DeleteApplication deletes an application.
func (r *SQLRepository) DeleteApplication(id string) error {
	_, err := r.db.Exec("DELETE FROM applications WHERE id = ?", id)
	return err
}

// OnboardApplicationToModule inserts an onboarding entry.
func (r *SQLRepository) OnboardApplicationToModule(moduleID, appCode string) error {
	_, err := r.db.Exec(`
		INSERT INTO module_applications (module_id, app_code, status) 
		VALUES (?, ?, 'active')
		ON CONFLICT(module_id, app_code) DO UPDATE SET status = 'active'`,
		moduleID, appCode)
	return err
}

// OffboardApplicationFromModule updates the onboarding entry status to 'inactive'.
func (r *SQLRepository) OffboardApplicationFromModule(moduleID, appCode string) error {
	_, err := r.db.Exec(`
		UPDATE module_applications SET status = 'inactive' WHERE module_id = ? AND app_code = ?`,
		moduleID, appCode)
	return err
}

// ListModuleApplications lists all active app codes onboarded to a module.
func (r *SQLRepository) ListModuleApplications(moduleID string) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT app_code FROM module_applications WHERE module_id = ? AND status = 'active'`,
		moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		list = append(list, code)
	}
	if list == nil {
		list = []string{}
	}
	return list, nil
}


