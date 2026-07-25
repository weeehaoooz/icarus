package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"icarus-auth-ms/internal/models"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// SeedDefaultRBAC seeds the default admin user.
func (r *SQLRepository) SeedDefaultRBAC() error {
	log.Println("Seeding default admin user...")

	var adminExists bool
	var err error
	if r.driver == "postgres" {
		err = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", "admin").Scan(&adminExists)
	} else {
		err = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", "admin").Scan(&adminExists)
	}
	if err != nil {
		return fmt.Errorf("failed to check admin user: %w", err)
	}

	var adminUserID int64
	if !adminExists {
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash admin password: %w", err)
		}
		passwordHash := string(hashedBytes)

		if r.driver == "postgres" {
			err = r.db.QueryRow(`
				INSERT INTO users (username, email, first_name, last_name, password_hash) 
				VALUES ($1, $2, $3, $4, $5) RETURNING id`,
				"admin", "admin@example.com", "System", "Admin", passwordHash).Scan(&adminUserID)
		} else {
			res, err := r.db.Exec(`
				INSERT INTO users (username, email, first_name, last_name, password_hash) 
				VALUES (?, ?, ?, ?, ?)`,
				"admin", "admin@example.com", "System", "Admin", passwordHash)
			if err == nil {
				adminUserID, err = res.LastInsertId()
			}
		}
		if err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}
		log.Printf("Seeded user admin (ID: %d) with password admin123", adminUserID)
	} else {
		if r.driver == "postgres" {
			err = r.db.QueryRow("SELECT id FROM users WHERE username = $1", "admin").Scan(&adminUserID)
		} else {
			err = r.db.QueryRow("SELECT id FROM users WHERE username = ?", "admin").Scan(&adminUserID)
		}
		if err != nil {
			return fmt.Errorf("failed to get existing admin user ID: %w", err)
		}
	}

	var userHasRole bool
	if r.driver == "postgres" {
		err = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM user_tenant_module_roles WHERE user_id = $1 AND role_id = $2)", adminUserID, "icarus-auth-ms:admin").Scan(&userHasRole)
	} else {
		err = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM user_tenant_module_roles WHERE user_id = ? AND role_id = ?)", adminUserID, "icarus-auth-ms:admin").Scan(&userHasRole)
	}
	if err != nil {
		return fmt.Errorf("failed to check admin user role mapping: %w", err)
	}

	if !userHasRole {
		if r.driver == "postgres" {
			_, err = r.db.Exec(`
				INSERT INTO user_tenant_module_roles (id, user_id, tenant_id, module_id, role_id) 
				VALUES ($1, $2, $3, $4, $5)`,
				"seed-user-admin-role", adminUserID, "system-tenant", "icarus-auth-ms", "icarus-auth-ms:admin")
		} else {
			_, err = r.db.Exec(`
				INSERT INTO user_tenant_module_roles (id, user_id, tenant_id, module_id, role_id) 
				VALUES (?, ?, ?, ?, ?)`,
				"seed-user-admin-role", adminUserID, "system-tenant", "icarus-auth-ms", "icarus-auth-ms:admin")
		}
		if err != nil {
			return fmt.Errorf("failed to assign admin role to admin user: %w", err)
		}
		log.Println("Assigned role admin to user admin")
	}

	log.Println("Admin user seeding completed successfully!")
	return nil
}

// ListUsers retrieves all users and their roles.
func (r *SQLRepository) ListUsers() ([]models.User, error) {
	query := `SELECT id, username, email, first_name, last_name, created_at FROM users ORDER BY id ASC`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		var emailVal, firstNameVal, lastNameVal sql.NullString

		if r.driver == "postgres" {
			err = rows.Scan(&u.ID, &u.Username, &emailVal, &firstNameVal, &lastNameVal, &u.CreatedAt)
		} else {
			var createdAtStr string
			err = rows.Scan(&u.ID, &u.Username, &emailVal, &firstNameVal, &lastNameVal, &createdAtStr)
			if err == nil {
				u.CreatedAt, _ = parseTime(createdAtStr)
			}
		}
		if err != nil {
			return nil, err
		}

		u.Email = emailVal.String
		u.FirstName = firstNameVal.String
		u.LastName = lastNameVal.String

		// Get user roles (returns role names)
		roles, err := r.GetUserRoles(u.ID)
		if err != nil {
			return nil, err
		}
		u.Roles = roles

		users = append(users, u)
	}

	return users, nil
}

// UpdateUser updates a user's details.
func (r *SQLRepository) UpdateUser(id int64, username, email, firstName, lastName, passwordHash string) error {
	var err error
	if passwordHash != "" {
		if r.driver == "postgres" {
			_, err = r.db.Exec(`
				UPDATE users SET username = $1, email = $2, first_name = $3, last_name = $4, password_hash = $5 
				WHERE id = $6`,
				username, email, firstName, lastName, passwordHash, id)
		} else {
			_, err = r.db.Exec(`
				UPDATE users SET username = ?, email = ?, first_name = ?, last_name = ?, password_hash = ? 
				WHERE id = ?`,
				username, email, firstName, lastName, passwordHash, id)
		}
	} else {
		if r.driver == "postgres" {
			_, err = r.db.Exec(`
				UPDATE users SET username = $1, email = $2, first_name = $3, last_name = $4 
				WHERE id = $5`,
				username, email, firstName, lastName, id)
		} else {
			_, err = r.db.Exec(`
				UPDATE users SET username = ?, email = ?, first_name = ?, last_name = ? 
				WHERE id = ?`,
				username, email, firstName, lastName, id)
		}
	}
	return err
}

// DeleteUser deletes a user.
func (r *SQLRepository) DeleteUser(id int64) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec("DELETE FROM users WHERE id = $1", id)
	} else {
		_, err = r.db.Exec("DELETE FROM users WHERE id = ?", id)
	}
	return err
}

// GetUserRoles returns all role names assigned to the user across all tenants/modules.
func (r *SQLRepository) GetUserRoles(userID int64) ([]string, error) {
	var rows *sql.Rows
	var err error

	if r.driver == "postgres" {
		rows, err = r.db.Query(`
			SELECT r.name 
			FROM user_tenant_module_roles utmr 
			JOIN roles r ON utmr.role_id = r.id 
			WHERE utmr.user_id = $1`, userID)
	} else {
		rows, err = r.db.Query(`
			SELECT r.name 
			FROM user_tenant_module_roles utmr 
			JOIN roles r ON utmr.role_id = r.id 
			WHERE utmr.user_id = ?`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	if roles == nil {
		roles = []string{}
	}
	return roles, nil
}

// GetUserPermissions gets all unique permission actions assigned to a user via their roles.
func (r *SQLRepository) GetUserPermissions(userID int64) ([]string, error) {
	var rows *sql.Rows
	var err error

	query := `
		SELECT DISTINCT p.action 
		FROM user_tenant_module_roles utmr
		JOIN role_permissions rp ON utmr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE utmr.user_id = `
	if r.driver == "postgres" {
		rows, err = r.db.Query(query+"$1", userID)
	} else {
		rows, err = r.db.Query(query+"?", userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		permissions = append(permissions, p)
	}
	if permissions == nil {
		permissions = []string{}
	}
	return permissions, nil
}

// AssignUserRoles assigns roles to a user across different modules (by resolving role names in the DB).
func (r *SQLRepository) AssignUserRoles(userID int64, roles []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if r.driver == "postgres" {
		_, err = tx.Exec("DELETE FROM user_tenant_module_roles WHERE user_id = $1 AND tenant_id = $2", userID, "system-tenant")
	} else {
		_, err = tx.Exec("DELETE FROM user_tenant_module_roles WHERE user_id = ? AND tenant_id = ?", userID, "system-tenant")
	}
	if err != nil {
		return err
	}

	for _, roleName := range roles {
		if roleName == "" {
			continue
		}
		var roleID string
		var moduleID string
		var findErr error
		if r.driver == "postgres" {
			findErr = tx.QueryRow("SELECT id, module_id FROM roles WHERE name = $1 LIMIT 1", roleName).Scan(&roleID, &moduleID)
		} else {
			findErr = tx.QueryRow("SELECT id, module_id FROM roles WHERE name = ? LIMIT 1", roleName).Scan(&roleID, &moduleID)
		}
		if findErr != nil {
			// If role doesn't exist, create it under auth-ms (fallback for legacy roles)
			roleID = "icarus-auth-ms:" + roleName
			moduleID = "icarus-auth-ms"
			if r.driver == "postgres" {
				_, err = tx.Exec("INSERT INTO roles (id, module_id, name, description) VALUES ($1, $2, $3, $4)", roleID, "icarus-auth-ms", roleName, "Auto-created legacy role")
			} else {
				_, err = tx.Exec("INSERT INTO roles (id, module_id, name, description) VALUES (?, ?, ?, ?)", roleID, "icarus-auth-ms", roleName, "Auto-created legacy role")
			}
			if err != nil {
				return err
			}
		}

		id := fmt.Sprintf("%d-%s", userID, roleID)
		if r.driver == "postgres" {
			_, err = tx.Exec(`
				INSERT INTO user_tenant_module_roles (id, user_id, tenant_id, module_id, role_id) 
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (user_id, tenant_id, module_id) DO UPDATE SET role_id = EXCLUDED.role_id`,
				id, userID, "system-tenant", moduleID, roleID)
		} else {
			_, err = tx.Exec(`
				INSERT INTO user_tenant_module_roles (id, user_id, tenant_id, module_id, role_id) 
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT (user_id, tenant_id, module_id) DO UPDATE SET role_id = excluded.role_id`,
				id, userID, "system-tenant", moduleID, roleID)
		}
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// AssignUserTenantModuleRole assigns a user to a specific role in a specific tenant and module context.
func (r *SQLRepository) AssignUserTenantModuleRole(userID int64, tenantID, moduleID, roleID string) error {
	id := fmt.Sprintf("%d-%s-%s", userID, tenantID, moduleID)
	var query string
	if r.driver == "postgres" {
		query = `
			INSERT INTO user_tenant_module_roles (id, user_id, tenant_id, module_id, role_id) 
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (user_id, tenant_id, module_id) DO UPDATE SET role_id = EXCLUDED.role_id`
		_, err := r.db.Exec(query, id, userID, tenantID, moduleID, roleID)
		return err
	} else {
		query = `
			INSERT INTO user_tenant_module_roles (id, user_id, tenant_id, module_id, role_id) 
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (user_id, tenant_id, module_id) DO UPDATE SET role_id = excluded.role_id`
		_, err := r.db.Exec(query, id, userID, tenantID, moduleID, roleID)
		return err
	}
}

// RevokeUserTenantModuleRole revokes a user's role assignment under a specific tenant and module.
func (r *SQLRepository) RevokeUserTenantModuleRole(userID int64, tenantID, moduleID string) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec("DELETE FROM user_tenant_module_roles WHERE user_id = $1 AND tenant_id = $2 AND module_id = $3", userID, tenantID, moduleID)
	} else {
		_, err = r.db.Exec("DELETE FROM user_tenant_module_roles WHERE user_id = ? AND tenant_id = ? AND module_id = ?", userID, tenantID, moduleID)
	}
	return err
}

// ListClients retrieves all clients.
func (r *SQLRepository) ListClients() ([]models.Client, error) {
	rows, err := r.db.Query("SELECT client_id, public_key, created_at FROM clients ORDER BY client_id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []models.Client
	for rows.Next() {
		var c models.Client
		if r.driver == "postgres" {
			err = rows.Scan(&c.ClientID, &c.PublicKey, &c.CreatedAt)
		} else {
			var createdAtStr string
			err = rows.Scan(&c.ClientID, &c.PublicKey, &createdAtStr)
			if err == nil {
				c.CreatedAt, _ = parseTime(createdAtStr)
			}
		}
		if err != nil {
			return nil, err
		}

		// Legacy: get roles
		roles, err := r.GetClientRoles(c.ClientID)
		if err != nil {
			roles = []string{}
		}
		c.Roles = roles

		clients = append(clients, c)
	}
	return clients, nil
}

// DeleteClient deletes a client.
func (r *SQLRepository) DeleteClient(clientID string) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec("DELETE FROM clients WHERE client_id = $1", clientID)
	} else {
		_, err = r.db.Exec("DELETE FROM clients WHERE client_id = ?", clientID)
	}
	return err
}

// GetClientRoles returns all role names assigned to the client.
func (r *SQLRepository) GetClientRoles(clientID string) ([]string, error) {
	var rows *sql.Rows
	var err error

	if r.driver == "postgres" {
		rows, err = r.db.Query(`
			SELECT r.name 
			FROM client_tenant_module_roles ctmr 
			JOIN roles r ON ctmr.role_id = r.id 
			WHERE ctmr.client_id = $1`, clientID)
	} else {
		rows, err = r.db.Query(`
			SELECT r.name 
			FROM client_tenant_module_roles ctmr 
			JOIN roles r ON ctmr.role_id = r.id 
			WHERE ctmr.client_id = ?`, clientID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	if roles == nil {
		roles = []string{}
	}
	return roles, nil
}

// GetClientPermissions gets all unique permissions assigned to a client via their roles.
func (r *SQLRepository) GetClientPermissions(clientID string) ([]string, error) {
	var rows *sql.Rows
	var err error

	query := `
		SELECT DISTINCT p.action 
		FROM client_tenant_module_roles ctmr
		JOIN role_permissions rp ON ctmr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE ctmr.client_id = `
	if r.driver == "postgres" {
		rows, err = r.db.Query(query+"$1", clientID)
	} else {
		rows, err = r.db.Query(query+"?", clientID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		permissions = append(permissions, p)
	}
	if permissions == nil {
		permissions = []string{}
	}
	return permissions, nil
}

// AssignClientRoles assigns roles to a client.
func (r *SQLRepository) AssignClientRoles(clientID string, roles []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if r.driver == "postgres" {
		_, err = tx.Exec("DELETE FROM client_tenant_module_roles WHERE client_id = $1 AND tenant_id = $2 AND module_id = $3", clientID, "system-tenant", "icarus-auth-ms")
	} else {
		_, err = tx.Exec("DELETE FROM client_tenant_module_roles WHERE client_id = ? AND tenant_id = ? AND module_id = ?", clientID, "system-tenant", "icarus-auth-ms")
	}
	if err != nil {
		return err
	}

	for _, roleName := range roles {
		if roleName == "" {
			continue
		}
		var roleID string
		var findErr error
		if r.driver == "postgres" {
			findErr = tx.QueryRow("SELECT id FROM roles WHERE name = $1 AND module_id = $2", roleName, "icarus-auth-ms").Scan(&roleID)
		} else {
			findErr = tx.QueryRow("SELECT id FROM roles WHERE name = ? AND module_id = ?", roleName, "icarus-auth-ms").Scan(&roleID)
		}
		if findErr != nil {
			roleID = "icarus-auth-ms:" + roleName
			if r.driver == "postgres" {
				_, err = tx.Exec("INSERT INTO roles (id, module_id, name, description) VALUES ($1, $2, $3, $4)", roleID, "icarus-auth-ms", roleName, "Auto-created legacy role")
			} else {
				_, err = tx.Exec("INSERT INTO roles (id, module_id, name, description) VALUES (?, ?, ?, ?)", roleID, "icarus-auth-ms", roleName, "Auto-created legacy role")
			}
			if err != nil {
				return err
			}
		}

		id := fmt.Sprintf("%s-%s", clientID, roleID)
		if r.driver == "postgres" {
			_, err = tx.Exec(`
				INSERT INTO client_tenant_module_roles (id, client_id, tenant_id, module_id, role_id) 
				VALUES ($1, $2, $3, $4, $5)`,
				id, clientID, "system-tenant", "icarus-auth-ms", roleID)
		} else {
			_, err = tx.Exec(`
				INSERT INTO client_tenant_module_roles (id, client_id, tenant_id, module_id, role_id) 
				VALUES (?, ?, ?, ?, ?)`,
				id, clientID, "system-tenant", "icarus-auth-ms", roleID)
		}
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListRoles retrieves all roles.
func (r *SQLRepository) ListRoles() ([]models.Role, error) {
	rows, err := r.db.Query("SELECT id, module_id, tenant_id, app_code, name, description, is_system_role, is_active, type, created_at FROM roles ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []models.Role
	for rows.Next() {
		var role models.Role
		var tenantID sql.NullString
		var desc sql.NullString
		var roleType sql.NullString
		var createdAtStr string

		err = rows.Scan(&role.ID, &role.ModuleID, &tenantID, &role.AppCode, &role.Name, &desc, &role.IsSystemRole, &role.IsActive, &roleType, &createdAtStr)
		if err != nil {
			return nil, err
		}
		role.Description = desc.String
		role.Type = roleType.String
		if role.Type == "" {
			role.Type = "Custom"
		}
		if tenantID.Valid {
			tVal := tenantID.String
			role.TenantID = &tVal
		}
		role.CreatedAt, _ = parseTime(createdAtStr)

		// Get mapped permissions
		perms, err := r.getRolePermissions(role.ID)
		if err != nil {
			return nil, err
		}
		role.Permissions = perms

		// Get nested roles
		nested, err := r.getNestedRoles(role.ID)
		if err != nil {
			return nil, err
		}
		role.NestedRoles = nested

		roles = append(roles, role)
	}
	return roles, nil
}

// ListRolesPaged retrieves a page of roles with optional search filter.
func (r *SQLRepository) ListRolesPaged(search string, limit, offset int) ([]models.Role, error) {
	var rows *sql.Rows
	var err error

	query := "SELECT id, module_id, tenant_id, app_code, name, description, is_system_role, is_active, type, created_at FROM roles"
	var args []interface{}

	if search != "" {
		query += " WHERE name LIKE ? OR description LIKE ?"
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	query += " ORDER BY name ASC"

	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	if r.driver == "postgres" {
		placeholderCount := 1
		for strings.Contains(query, "?") {
			placeholder := fmt.Sprintf("$%d", placeholderCount)
			query = strings.Replace(query, "?", placeholder, 1)
			placeholderCount++
		}
	}

	rows, err = r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []models.Role
	for rows.Next() {
		var role models.Role
		var tenantID sql.NullString
		var desc sql.NullString
		var roleType sql.NullString
		var createdAtStr string

		err = rows.Scan(&role.ID, &role.ModuleID, &tenantID, &role.AppCode, &role.Name, &desc, &role.IsSystemRole, &role.IsActive, &roleType, &createdAtStr)
		if err != nil {
			return nil, err
		}
		role.Description = desc.String
		role.Type = roleType.String
		if role.Type == "" {
			role.Type = "Custom"
		}
		if tenantID.Valid {
			tVal := tenantID.String
			role.TenantID = &tVal
		}
		role.CreatedAt, _ = parseTime(createdAtStr)

		// Get mapped permissions
		perms, err := r.getRolePermissions(role.ID)
		if err != nil {
			return nil, err
		}
		role.Permissions = perms

		// Get nested roles
		nested, err := r.getNestedRoles(role.ID)
		if err != nil {
			return nil, err
		}
		role.NestedRoles = nested

		roles = append(roles, role)
	}
	return roles, nil
}

func (r *SQLRepository) getNestedRoles(roleID string) ([]string, error) {
	var rows *sql.Rows
	var err error
	if r.driver == "postgres" {
		rows, err = r.db.Query(`
			SELECT r.name 
			FROM role_hierarchy rh 
			JOIN roles r ON rh.child_role_id = r.id 
			WHERE rh.parent_role_id = $1 
			ORDER BY r.name ASC`, roleID)
	} else {
		rows, err = r.db.Query(`
			SELECT r.name 
			FROM role_hierarchy rh 
			JOIN roles r ON rh.child_role_id = r.id 
			WHERE rh.parent_role_id = ? 
			ORDER BY r.name ASC`, roleID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	if roles == nil {
		roles = []string{}
	}
	return roles, nil
}

func (r *SQLRepository) getRolePermissions(roleID string) ([]string, error) {
	var rows *sql.Rows
	var err error
	if r.driver == "postgres" {
		rows, err = r.db.Query("SELECT p.action FROM role_permissions rp JOIN permissions p ON rp.permission_id = p.id WHERE rp.role_id = $1", roleID)
	} else {
		rows, err = r.db.Query("SELECT p.action FROM role_permissions rp JOIN permissions p ON rp.permission_id = p.id WHERE rp.role_id = ?", roleID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	if perms == nil {
		perms = []string{}
	}
	return perms, nil
}

// CreateRole creates a new role.
func (r *SQLRepository) CreateRole(moduleID, appCode, name, description, roleType string) error {
	if roleType == "" {
		roleType = "Custom"
	}
	if moduleID == "" {
		moduleID = "icarus-auth-ms"
	}
	id := moduleID + ":" + name
	if appCode != "" {
		id = moduleID + ":" + appCode + ":" + name
	}
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec(`
			INSERT INTO roles (id, module_id, tenant_id, app_code, name, description, is_system_role, type) 
			VALUES ($1, $2, NULL, $3, $4, $5, 0, $6)`,
			id, moduleID, appCode, name, description, roleType)
	} else {
		_, err = r.db.Exec(`
			INSERT INTO roles (id, module_id, tenant_id, app_code, name, description, is_system_role, type) 
			VALUES (?, ?, NULL, ?, ?, ?, 0, ?)`,
			id, moduleID, appCode, name, description, roleType)
	}
	return err
}

// CreateScopedRole creates a role scoped to a specific tenant and module.
func (r *SQLRepository) CreateScopedRole(id, moduleID string, tenantID *string, appCode, name, description, roleType string, isSystem bool) error {
	if roleType == "" {
		roleType = "Custom"
	}
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec(`
			INSERT INTO roles (id, module_id, tenant_id, app_code, name, description, is_system_role, type) 
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, moduleID, tenantID, appCode, name, description, isSystem, roleType)
	} else {
		_, err = r.db.Exec(`
			INSERT INTO roles (id, module_id, tenant_id, app_code, name, description, is_system_role, type) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, moduleID, tenantID, appCode, name, description, isSystem, roleType)
	}
	return err
}

// UpdateRole updates a role description, type, mapped permissions, and nested roles.
func (r *SQLRepository) UpdateRole(id, description, roleType string, isActive bool, permissions []string, nestedRoles []string) error {
	if roleType == "" {
		roleType = "Custom"
	}
	roleID := id

	var exists bool
	var checkErr error
	if r.driver == "postgres" {
		checkErr = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1)", roleID).Scan(&exists)
	} else {
		checkErr = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM roles WHERE id = ?)", roleID).Scan(&exists)
	}
	if checkErr != nil || !exists {
		var resolvedID string
		var nameErr error
		if r.driver == "postgres" {
			nameErr = r.db.QueryRow("SELECT id FROM roles WHERE name = $1", id).Scan(&resolvedID)
		} else {
			nameErr = r.db.QueryRow("SELECT id FROM roles WHERE name = ?", id).Scan(&resolvedID)
		}
		if nameErr == nil {
			roleID = resolvedID
		}
	}

	var name string
	if r.driver == "postgres" {
		_ = r.db.QueryRow("SELECT name FROM roles WHERE id = $1", roleID).Scan(&name)
	} else {
		_ = r.db.QueryRow("SELECT name FROM roles WHERE id = ?", roleID).Scan(&name)
	}

	// Fetch current hierarchy to check for cycles
	hierarchy, err := r.GetRoleHierarchy()
	if err != nil {
		return err
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Resolve nested roles to IDs
	resolvedNestedRoleIDs := make([]string, 0, len(nestedRoles))
	for _, nr := range nestedRoles {
		if nr == "" || nr == name || nr == roleID {
			continue
		}
		var childRoleID string
		var findErr error
		if r.driver == "postgres" {
			findErr = tx.QueryRow("SELECT id FROM roles WHERE name = $1 OR id = $2", nr, nr).Scan(&childRoleID)
		} else {
			findErr = tx.QueryRow("SELECT id FROM roles WHERE name = ? OR id = ?", nr, nr).Scan(&childRoleID)
		}
		if findErr == nil {
			resolvedNestedRoleIDs = append(resolvedNestedRoleIDs, childRoleID)
		} else {
			resolvedNestedRoleIDs = append(resolvedNestedRoleIDs, nr)
		}
	}

	if hasCycle(roleID, resolvedNestedRoleIDs, hierarchy) {
		return errors.New("invalid role hierarchy: recursive nesting detected")
	}

	// Update description, type, and active status
	if r.driver == "postgres" {
		_, err = tx.Exec("UPDATE roles SET description = $1, type = $2, is_active = $3 WHERE id = $4", description, roleType, isActive, roleID)
	} else {
		_, err = tx.Exec("UPDATE roles SET description = ?, type = ?, is_active = ? WHERE id = ?", description, roleType, isActive, roleID)
	}
	if err != nil {
		return err
	}

	// Delete existing permission mappings
	if r.driver == "postgres" {
		_, err = tx.Exec("DELETE FROM role_permissions WHERE role_id = $1", roleID)
	} else {
		_, err = tx.Exec("DELETE FROM role_permissions WHERE role_id = ?", roleID)
	}
	if err != nil {
		return err
	}

	// Map new permissions (resolve action string to permission_id)
	for _, p := range permissions {
		if p == "" {
			continue
		}
		var permID string
		var getErr error
		if r.driver == "postgres" {
			getErr = tx.QueryRow("SELECT id FROM permissions WHERE action = $1 OR id = $2", p, p).Scan(&permID)
		} else {
			getErr = tx.QueryRow("SELECT id FROM permissions WHERE action = ? OR id = ?", p, p).Scan(&permID)
		}
		if getErr != nil {
			// Auto create permission under auth-ms for backward compatibility
			permID = "icarus-auth-ms:" + p
			if r.driver == "postgres" {
				_, err = tx.Exec("INSERT INTO permissions (id, module_id, action, description) VALUES ($1, $2, $3, $4)", permID, "icarus-auth-ms", p, "Auto-created legacy permission")
			} else {
				_, err = tx.Exec("INSERT INTO permissions (id, module_id, action, description) VALUES (?, ?, ?, ?)", permID, "icarus-auth-ms", p, "Auto-created legacy permission")
			}
			if err != nil {
				return err
			}
		}

		if r.driver == "postgres" {
			_, err = tx.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)", roleID, permID)
		} else {
			_, err = tx.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", roleID, permID)
		}
		if err != nil {
			return err
		}
	}

	// Delete existing nested roles
	if r.driver == "postgres" {
		_, err = tx.Exec("DELETE FROM role_hierarchy WHERE parent_role_id = $1", roleID)
	} else {
		_, err = tx.Exec("DELETE FROM role_hierarchy WHERE parent_role_id = ?", roleID)
	}
	if err != nil {
		return err
	}

	// Map new nested roles
	for _, childRoleID := range resolvedNestedRoleIDs {
		if r.driver == "postgres" {
			_, err = tx.Exec("INSERT INTO role_hierarchy (parent_role_id, child_role_id) VALUES ($1, $2)", roleID, childRoleID)
		} else {
			_, err = tx.Exec("INSERT INTO role_hierarchy (parent_role_id, child_role_id) VALUES (?, ?)", roleID, childRoleID)
		}
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func hasCycle(parent string, children []string, hierarchy map[string][]string) bool {
	visited := make(map[string]bool)
	var detectCycle func(node string) bool
	detectCycle = func(node string) bool {
		if node == parent {
			return true
		}
		if visited[node] {
			return false
		}
		visited[node] = true
		for _, child := range hierarchy[node] {
			if detectCycle(child) {
				return true
			}
		}
		return false
	}

	for _, child := range children {
		visited = make(map[string]bool)
		// Translate name to ID if needed
		if detectCycle(child) {
			return true
		}
	}
	return false
}

// DeleteRole deletes a role.
func (r *SQLRepository) DeleteRole(id string) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec("DELETE FROM roles WHERE id = $1 OR name = $2", id, id)
	} else {
		_, err = r.db.Exec("DELETE FROM roles WHERE id = ? OR name = ?", id, id)
	}
	return err
}

// ListPermissions retrieves all permissions.
func (r *SQLRepository) ListPermissions() ([]models.Permission, error) {
	rows, err := r.db.Query("SELECT id, module_id, action, description, created_at FROM permissions ORDER BY action ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []models.Permission
	for rows.Next() {
		var p models.Permission
		var desc sql.NullString
		var createdAtStr string
		err = rows.Scan(&p.ID, &p.ModuleID, &p.Action, &desc, &createdAtStr)
		if err != nil {
			return nil, err
		}
		p.Description = desc.String
		p.CreatedAt, _ = parseTime(createdAtStr)
		permissions = append(permissions, p)
	}
	return permissions, nil
}


// SyncLDAPGroups synchronizes a user's LDAP group memberships.
func (r *SQLRepository) SyncLDAPGroups(userID int64, groupNames []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if r.driver == "postgres" {
		_, err = tx.Exec("DELETE FROM user_groups WHERE user_id = $1 AND group_type = 'LDAP'", userID)
	} else {
		_, err = tx.Exec("DELETE FROM user_groups WHERE user_id = ? AND group_type = 'LDAP'", userID)
	}
	if err != nil {
		return err
	}

	for _, name := range groupNames {
		if name == "" {
			continue
		}
		if r.driver == "postgres" {
			_, err = tx.Exec("INSERT INTO user_groups (user_id, group_name, group_type) VALUES ($1, $2, 'LDAP')", userID, name)
		} else {
			_, err = tx.Exec("INSERT INTO user_groups (user_id, group_name, group_type) VALUES (?, ?, 'LDAP')", userID, name)
		}
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// AssignUserCustomGroups assigns a user to custom LDAP/non-LDAP groups.
func (r *SQLRepository) AssignUserCustomGroups(userID int64, groupNames []string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if r.driver == "postgres" {
		_, err = tx.Exec("DELETE FROM user_groups WHERE user_id = $1 AND group_type = 'Custom'", userID)
	} else {
		_, err = tx.Exec("DELETE FROM user_groups WHERE user_id = ? AND group_type = 'Custom'", userID)
	}
	if err != nil {
		return err
	}

	for _, name := range groupNames {
		if name == "" {
			continue
		}
		if r.driver == "postgres" {
			_, err = tx.Exec("INSERT INTO user_groups (user_id, group_name, group_type) VALUES ($1, $2, 'Custom')", userID, name)
		} else {
			_, err = tx.Exec("INSERT INTO user_groups (user_id, group_name, group_type) VALUES (?, ?, 'Custom')", userID, name)
		}
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetRoleHierarchy returns the map of parent role IDs to their child role IDs.
func (r *SQLRepository) GetRoleHierarchy() (map[string][]string, error) {
	rows, err := r.db.Query("SELECT parent_role_id, child_role_id FROM role_hierarchy")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hierarchy := make(map[string][]string)
	for rows.Next() {
		var parent, child string
		if err := rows.Scan(&parent, &child); err != nil {
			return nil, err
		}
		hierarchy[parent] = append(hierarchy[parent], child)
	}
	return hierarchy, nil
}

// resolveRoles transitively resolves all nested roles using DFS to avoid cycles.
func resolveRoles(startingRoles []string, hierarchy map[string][]string) []string {
	visited := make(map[string]bool)
	var result []string

	var dfs func(role string)
	dfs = func(role string) {
		if visited[role] {
			return
		}
		visited[role] = true
		result = append(result, role)
		for _, child := range hierarchy[role] {
			dfs(child)
		}
	}

	for _, role := range startingRoles {
		dfs(role)
	}
	return result
}

// GetPermissionsForRoles retrieves all unique permissions assigned to any of the listed roles.
func (r *SQLRepository) GetPermissionsForRoles(roleIDs []string) ([]string, error) {
	if len(roleIDs) == 0 {
		return []string{}, nil
	}

	var query string
	var args []interface{}
	placeholders := make([]string, len(roleIDs))

	if r.driver == "postgres" {
		for i, id := range roleIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args = append(args, id)
		}
	} else {
		for i, id := range roleIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
	}
	query = fmt.Sprintf("SELECT DISTINCT p.action FROM role_permissions rp JOIN permissions p ON rp.permission_id = p.id WHERE rp.role_id IN (%s)", strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var perm string
		if err := rows.Scan(&perm); err != nil {
			return nil, err
		}
		permissions = append(permissions, perm)
	}
	if permissions == nil {
		permissions = []string{}
	}
	return permissions, nil
}

// GetResolvedUserRolesAndPermissions recursively resolves all user roles and permissions scoped to a tenant and module.
func (r *SQLRepository) GetResolvedUserRolesAndPermissions(userID int64, tenantIDOrCode, moduleCode string) ([]string, []string, error) {
	var roleID string
	var query string
	if r.driver == "postgres" {
		query = `
			SELECT utmr.role_id 
			FROM user_tenant_module_roles utmr
			JOIN modules m ON utmr.module_id = m.id
			JOIN tenants t ON utmr.tenant_id = t.id
			WHERE utmr.user_id = $1 AND (t.id = $2 OR t.code = $3) AND m.code = $4`
		err := r.db.QueryRow(query, userID, tenantIDOrCode, tenantIDOrCode, moduleCode).Scan(&roleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return []string{}, []string{}, nil
			}
			return nil, nil, err
		}
	} else {
		query = `
			SELECT utmr.role_id 
			FROM user_tenant_module_roles utmr
			JOIN modules m ON utmr.module_id = m.id
			JOIN tenants t ON utmr.tenant_id = t.id
			WHERE utmr.user_id = ? AND (t.id = ? OR t.code = ?) AND m.code = ?`
		err := r.db.QueryRow(query, userID, tenantIDOrCode, tenantIDOrCode, moduleCode).Scan(&roleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return []string{}, []string{}, nil
			}
			return nil, nil, err
		}
	}

	hierarchy, err := r.GetRoleHierarchy()
	if err != nil {
		return nil, nil, err
	}

	resolvedRoleIDs := resolveRoles([]string{roleID}, hierarchy)

	var resolvedRoleNames []string
	if len(resolvedRoleIDs) > 0 {
		placeholders := make([]string, len(resolvedRoleIDs))
		args := make([]interface{}, len(resolvedRoleIDs))
		for i, id := range resolvedRoleIDs {
			if r.driver == "postgres" {
				placeholders[i] = fmt.Sprintf("$%d", i+1)
			} else {
				placeholders[i] = "?"
			}
			args[i] = id
		}
		roleQuery := fmt.Sprintf("SELECT name FROM roles WHERE id IN (%s)", strings.Join(placeholders, ","))
		rows, err := r.db.Query(roleQuery, args...)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err == nil {
				resolvedRoleNames = append(resolvedRoleNames, name)
			}
		}
	}

	permissions, err := r.GetPermissionsForRoles(resolvedRoleIDs)
	if err != nil {
		return nil, nil, err
	}

	return resolvedRoleNames, permissions, nil
}

// SyncModule registration function to save module, permissions, and roles.
func (r *SQLRepository) SyncModule(code, name, baseURL string, perms []models.Permission, defaultRoles []models.Role, appCentricRoles []models.Role, creator string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	moduleID := code // use code as module ID for simplicity

	// Upsert module
	if r.driver == "postgres" {
		_, err = tx.Exec(`
			INSERT INTO modules (id, code, name, base_url, is_active) 
			VALUES ($1, $2, $3, $4, 1)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, base_url = EXCLUDED.base_url`,
			moduleID, code, name, baseURL)
	} else {
		_, err = tx.Exec(`
			INSERT INTO modules (id, code, name, base_url, is_active) 
			VALUES (?, ?, ?, ?, 1)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name, base_url = excluded.base_url`,
			moduleID, code, name, baseURL)
	}
	if err != nil {
		return err
	}

	// Delete old roles & permissions for this module
	// They will be recreated based on the manifest
	if r.driver == "postgres" {
		_, err = tx.Exec("DELETE FROM permissions WHERE module_id = $1", moduleID)
	} else {
		_, err = tx.Exec("DELETE FROM permissions WHERE module_id = ?", moduleID)
	}
	if err != nil {
		return err
	}

	// Insert permissions
	for _, p := range perms {
		permID := moduleID + ":" + p.Action
		if r.driver == "postgres" {
			_, err = tx.Exec("INSERT INTO permissions (id, module_id, action, description) VALUES ($1, $2, $3, $4)", permID, moduleID, p.Action, p.Description)
		} else {
			_, err = tx.Exec("INSERT INTO permissions (id, module_id, action, description) VALUES (?, ?, ?, ?)", permID, moduleID, p.Action, p.Description)
		}
		if err != nil {
			return err
		}
	}

	// Automatically ensure a "<module_code> owner" role exists in defaultRoles
	ownerRoleName := fmt.Sprintf("%s owner", moduleID)
	hasOwnerRole := false
	for _, dr := range defaultRoles {
		if dr.Name == ownerRoleName {
			hasOwnerRole = true
			break
		}
	}
	if !hasOwnerRole {
		var actions []string
		for _, p := range perms {
			actions = append(actions, p.Action)
		}
		defaultRoles = append(defaultRoles, models.Role{
			Name:        ownerRoleName,
			Description: fmt.Sprintf("Module owner with full access to %s", moduleID),
			Permissions: actions,
		})
	}

	// Delete old default/system roles for this module (excluding app-centric ones)
	if r.driver == "postgres" {
		_, _ = tx.Exec(`
			DELETE FROM role_permissions 
			WHERE role_id IN (
				SELECT id FROM roles 
				WHERE module_id = $1 AND tenant_id IS NULL AND is_system_role = 1 AND (app_code IS NULL OR app_code = '')
			)`, moduleID)
		_, _ = tx.Exec(`
			DELETE FROM roles 
			WHERE module_id = $1 AND tenant_id IS NULL AND is_system_role = 1 AND (app_code IS NULL OR app_code = '')`, moduleID)
	} else {
		_, _ = tx.Exec(`
			DELETE FROM role_permissions 
			WHERE role_id IN (
				SELECT id FROM roles 
				WHERE module_id = ? AND tenant_id IS NULL AND is_system_role = 1 AND (app_code IS NULL OR app_code = '')
			)`, moduleID)
		_, _ = tx.Exec(`
			DELETE FROM roles 
			WHERE module_id = ? AND tenant_id IS NULL AND is_system_role = 1 AND (app_code IS NULL OR app_code = '')`, moduleID)
	}

	// Insert default roles and their permission mappings
	for _, dr := range defaultRoles {
		roleID := moduleID + ":" + dr.Name

		// Delete existing role mapping (to handle updates)
		if r.driver == "postgres" {
			_, _ = tx.Exec("DELETE FROM role_permissions WHERE role_id = $1", roleID)
			_, _ = tx.Exec("DELETE FROM roles WHERE id = $1", roleID)
		} else {
			_, _ = tx.Exec("DELETE FROM role_permissions WHERE role_id = ?", roleID)
			_, _ = tx.Exec("DELETE FROM roles WHERE id = ?", roleID)
		}

		if r.driver == "postgres" {
			_, err = tx.Exec(`
				INSERT INTO roles (id, module_id, tenant_id, name, description, is_system_role) 
				VALUES ($1, $2, NULL, $3, $4, 1)`,
				roleID, moduleID, dr.Name, dr.Description)
		} else {
			_, err = tx.Exec(`
				INSERT INTO roles (id, module_id, tenant_id, name, description, is_system_role) 
				VALUES (?, ?, NULL, ?, ?, 1)`,
				roleID, moduleID, dr.Name, dr.Description)
		}
		if err != nil {
			return err
		}

		// Map permissions to this role
		for _, action := range dr.Permissions {
			permID := moduleID + ":" + action
			if r.driver == "postgres" {
				_, err = tx.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)", roleID, permID)
			} else {
				_, err = tx.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", roleID, permID)
			}
			if err != nil {
				return err
			}
		}
	}

	// Delete old app centric templates for this module
	if r.driver == "postgres" {
		_, _ = tx.Exec("DELETE FROM app_centric_role_templates WHERE module_id = $1", moduleID)
	} else {
		_, _ = tx.Exec("DELETE FROM app_centric_role_templates WHERE module_id = ?", moduleID)
	}

	// Insert new app centric templates
	for _, acr := range appCentricRoles {
		templateID := moduleID + ":" + acr.Name
		if r.driver == "postgres" {
			_, err = tx.Exec(`
				INSERT INTO app_centric_role_templates (id, module_id, name, description) 
				VALUES ($1, $2, $3, $4)`,
				templateID, moduleID, acr.Name, acr.Description)
		} else {
			_, err = tx.Exec(`
				INSERT INTO app_centric_role_templates (id, module_id, name, description) 
				VALUES (?, ?, ?, ?)`,
				templateID, moduleID, acr.Name, acr.Description)
		}
		if err != nil {
			return err
		}

		for _, action := range acr.Permissions {
			permID := moduleID + ":" + action
			if r.driver == "postgres" {
				_, err = tx.Exec("INSERT INTO app_centric_role_template_permissions (template_id, permission_id) VALUES ($1, $2)", templateID, permID)
			} else {
				_, err = tx.Exec("INSERT INTO app_centric_role_template_permissions (template_id, permission_id) VALUES (?, ?)", templateID, permID)
			}
			if err != nil {
				return err
			}
		}
	}

	// Assign Module owner role to creator if provided
	if creator != "" {
		var userID int64
		var findErr error
		if r.driver == "postgres" {
			findErr = tx.QueryRow("SELECT id FROM users WHERE username = $1", creator).Scan(&userID)
		} else {
			findErr = tx.QueryRow("SELECT id FROM users WHERE username = ?", creator).Scan(&userID)
		}
		if findErr == nil {
			roleID := moduleID + ":" + ownerRoleName
			id := fmt.Sprintf("%d-%s", userID, roleID)
			if r.driver == "postgres" {
				_, err = tx.Exec(`
					INSERT INTO user_tenant_module_roles (id, user_id, tenant_id, module_id, role_id) 
					VALUES ($1, $2, $3, $4, $5)
					ON CONFLICT (user_id, tenant_id, module_id) DO UPDATE SET role_id = EXCLUDED.role_id`,
					id, userID, "system-tenant", moduleID, roleID)
			} else {
				_, err = tx.Exec(`
					INSERT INTO user_tenant_module_roles (id, user_id, tenant_id, module_id, role_id) 
					VALUES (?, ?, ?, ?, ?)
					ON CONFLICT (user_id, tenant_id, module_id) DO UPDATE SET role_id = excluded.role_id`,
					id, userID, "system-tenant", moduleID, roleID)
			}
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// GetUserOwnedModules retrieves all module IDs where the user is a Module owner.
func (r *SQLRepository) GetUserOwnedModules(userID int64) ([]string, error) {
	query := `
		SELECT DISTINCT utmr.module_id 
		FROM user_tenant_module_roles utmr
		JOIN roles r ON utmr.role_id = r.id
		WHERE utmr.user_id = ? AND r.name = utmr.module_id || ' owner'`
	if r.driver == "postgres" {
		query = `
			SELECT DISTINCT utmr.module_id 
			FROM user_tenant_module_roles utmr
			JOIN roles r ON utmr.role_id = r.id
			WHERE utmr.user_id = $1 AND r.name = utmr.module_id || ' owner'`
	}

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var modules []string
	for rows.Next() {
		var modID string
		if err := rows.Scan(&modID); err == nil {
			modules = append(modules, modID)
		}
	}
	if modules == nil {
		modules = []string{}
	}
	return modules, nil
}

// GetModuleOwners retrieves usernames of all users holding the 'Module owner' role for a module.
func (r *SQLRepository) GetModuleOwners(moduleCode string) ([]string, error) {
	query := `
		SELECT DISTINCT u.username 
		FROM user_tenant_module_roles utmr
		JOIN users u ON utmr.user_id = u.id
		JOIN roles r ON utmr.role_id = r.id
		WHERE utmr.module_id = ? AND r.name = utmr.module_id || ' owner'`
	if r.driver == "postgres" {
		query = `
			SELECT DISTINCT u.username 
			FROM user_tenant_module_roles utmr
			JOIN users u ON utmr.user_id = u.id
			JOIN roles r ON utmr.role_id = r.id
			WHERE utmr.module_id = $1 AND r.name = utmr.module_id || ' owner'`
	}

	rows, err := r.db.Query(query, moduleCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var owners []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err == nil {
			owners = append(owners, username)
		}
	}
	if owners == nil {
		owners = []string{}
	}
	return owners, nil
}

// CreateTenant creates a new tenant.
func (r *SQLRepository) CreateTenant(id, code, name, status string) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec(`
			INSERT INTO tenants (id, code, name, status) 
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, status = EXCLUDED.status`,
			id, code, name, status)
	} else {
		_, err = r.db.Exec(`
			INSERT INTO tenants (id, code, name, status) 
			VALUES (?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name, status = excluded.status`,
			id, code, name, status)
	}
	return err
}

// DeleteTenant deletes a tenant and cascades.
func (r *SQLRepository) DeleteTenant(id string) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec("DELETE FROM tenants WHERE id = $1", id)
	} else {
		_, err = r.db.Exec("DELETE FROM tenants WHERE id = ?", id)
	}
	return err
}

// DeleteModule deletes a module and cascades.
func (r *SQLRepository) DeleteModule(id string) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec("DELETE FROM modules WHERE id = $1", id)
	} else {
		_, err = r.db.Exec("DELETE FROM modules WHERE id = ?", id)
	}
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
		if r.driver == "postgres" {
			var t time.Time
			err = rows.Scan(&app.ID, &app.Code, &app.Name, &desc, &t)
			if err == nil {
				app.CreatedAt = t
			}
		} else {
			err = rows.Scan(&app.ID, &app.Code, &app.Name, &desc, &createdAtStr)
			if err == nil {
				app.CreatedAt, _ = parseTime(createdAtStr)
			}
		}
		if err != nil {
			return nil, err
		}
		app.Description = desc.String
		list = append(list, app)
	}
	return list, nil
}

// CreateApplication creates or updates an application.
func (r *SQLRepository) CreateApplication(id, code, name, description string) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec(`
			INSERT INTO applications (id, code, name, description) 
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description`,
			id, code, name, description)
	} else {
		_, err = r.db.Exec(`
			INSERT INTO applications (id, code, name, description) 
			VALUES (?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name, description = excluded.description`,
			id, code, name, description)
	}
	return err
}

// DeleteApplication deletes an application.
func (r *SQLRepository) DeleteApplication(id string) error {
	var err error
	if r.driver == "postgres" {
		_, err = r.db.Exec("DELETE FROM applications WHERE id = $1", id)
	} else {
		_, err = r.db.Exec("DELETE FROM applications WHERE id = ?", id)
	}
	return err
}

// OnboardApplicationToModule registers application onboarding to a module and generates app-centric roles.
func (r *SQLRepository) OnboardApplicationToModule(moduleID, appCode string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Insert or update the onboarding association status
	if r.driver == "postgres" {
		_, err = tx.Exec(`
			INSERT INTO module_applications (module_id, app_code, status) 
			VALUES ($1, $2, 'active')
			ON CONFLICT (module_id, app_code) DO UPDATE SET status = 'active'`,
			moduleID, appCode)
	} else {
		_, err = tx.Exec(`
			INSERT INTO module_applications (module_id, app_code, status) 
			VALUES (?, ?, 'active')
			ON CONFLICT (module_id, app_code) DO UPDATE SET status = 'active'`,
			moduleID, appCode)
	}
	if err != nil {
		return err
	}

	// 2. Query all templates for this module
	type tempRole struct {
		id          string
		name        string
		description string
	}
	var templates []tempRole
	rows, err := tx.Query("SELECT id, name, description FROM app_centric_role_templates WHERE module_id = ?", moduleID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var tr tempRole
		if err := rows.Scan(&tr.id, &tr.name, &tr.description); err == nil {
			templates = append(templates, tr)
		}
	}
	rows.Close()

	// 3. For each template, instantiate the app-centric role
	for _, tr := range templates {
		// Appended APP Code BEFORE the module's app centric role: appCode + "-" + name
		roleName := appCode + "-" + tr.name
		roleID := moduleID + ":" + appCode + ":" + tr.name

		// Upsert the role and ensure it is active
		if r.driver == "postgres" {
			_, err = tx.Exec(`
				INSERT INTO roles (id, module_id, tenant_id, app_code, name, description, is_system_role, is_active) 
				VALUES ($1, $2, NULL, $3, $4, $5, 1, 1)
				ON CONFLICT (id) DO UPDATE SET is_active = 1, description = EXCLUDED.description`,
				roleID, moduleID, appCode, roleName, tr.description)
		} else {
			_, err = tx.Exec(`
				INSERT INTO roles (id, module_id, tenant_id, app_code, name, description, is_system_role, is_active) 
				VALUES (?, ?, NULL, ?, ?, ?, 1, 1)
				ON CONFLICT (id) DO UPDATE SET is_active = 1, description = excluded.description`,
				roleID, moduleID, appCode, roleName, tr.description)
		}
		if err != nil {
			return err
		}

		// Delete existing permissions for this instantiated role
		_, _ = tx.Exec("DELETE FROM role_permissions WHERE role_id = ?", roleID)

		// Query and map permissions from template
		pRows, err := tx.Query("SELECT permission_id FROM app_centric_role_template_permissions WHERE template_id = ?", tr.id)
		if err != nil {
			return err
		}
		var permIDs []string
		for pRows.Next() {
			var pid string
			if err := pRows.Scan(&pid); err == nil {
				permIDs = append(permIDs, pid)
			}
		}
		pRows.Close()

		for _, pid := range permIDs {
			if r.driver == "postgres" {
				_, err = tx.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)", roleID, pid)
			} else {
				_, err = tx.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", roleID, pid)
			}
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// OffboardApplicationFromModule sets onboarding association status to inactive and disables all app roles.
func (r *SQLRepository) OffboardApplicationFromModule(moduleID, appCode string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update onboarding status
	_, err = tx.Exec("UPDATE module_applications SET status = 'inactive' WHERE module_id = ? AND app_code = ?", moduleID, appCode)
	if err != nil {
		return err
	}

	// Disable the module's app roles for this application
	_, err = tx.Exec("UPDATE roles SET is_active = 0 WHERE module_id = ? AND app_code = ?", moduleID, appCode)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ListModuleApplications lists active application codes onboarded to a module.
func (r *SQLRepository) ListModuleApplications(moduleID string) ([]string, error) {
	rows, err := r.db.Query("SELECT app_code FROM module_applications WHERE module_id = ? AND status = 'active'", moduleID)
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

// GetModuleDefaultRoles returns the default roles (tenant_id IS NULL and is_system_role = 1 and app_code IS NULL/empty) for a module.
func (r *SQLRepository) GetModuleDefaultRoles(moduleID string) ([]models.Role, error) {
	var rows *sql.Rows
	var err error
	if r.driver == "postgres" {
		rows, err = r.db.Query(`
			SELECT id, module_id, name, description, is_system_role 
			FROM roles 
			WHERE module_id = $1 AND tenant_id IS NULL AND is_system_role = 1 AND (app_code IS NULL OR app_code = '')
		`, moduleID)
	} else {
		rows, err = r.db.Query(`
			SELECT id, module_id, name, description, is_system_role 
			FROM roles 
			WHERE module_id = ? AND tenant_id IS NULL AND is_system_role = 1 AND (app_code IS NULL OR app_code = '')
		`, moduleID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []models.Role
	for rows.Next() {
		var role models.Role
		var desc sql.NullString
		err = rows.Scan(&role.ID, &role.ModuleID, &role.Name, &desc, &role.IsSystemRole)
		if err != nil {
			return nil, err
		}
		role.Description = desc.String

		// Get mapped permissions
		perms, err := r.getRolePermissions(role.ID)
		if err != nil {
			return nil, err
		}
		role.Permissions = perms

		roles = append(roles, role)
	}
	if roles == nil {
		roles = []models.Role{}
	}
	return roles, nil
}

// GetModuleAppCentricRoles returns the app centric role templates for a module.
func (r *SQLRepository) GetModuleAppCentricRoles(moduleID string) ([]models.Role, error) {
	var rows *sql.Rows
	var err error
	if r.driver == "postgres" {
		rows, err = r.db.Query(`
			SELECT id, name, description 
			FROM app_centric_role_templates 
			WHERE module_id = $1
		`, moduleID)
	} else {
		rows, err = r.db.Query(`
			SELECT id, name, description 
			FROM app_centric_role_templates 
			WHERE module_id = ?
		`, moduleID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []models.Role
	for rows.Next() {
		var t models.Role
		var id string
		var desc sql.NullString
		err = rows.Scan(&id, &t.Name, &desc)
		if err != nil {
			return nil, err
		}
		t.ID = id
		t.ModuleID = moduleID
		t.Description = desc.String

		// Get mapped permissions for this template
		var pRows *sql.Rows
		if r.driver == "postgres" {
			pRows, err = r.db.Query(`
				SELECT p.action 
				FROM app_centric_role_template_permissions rp 
				JOIN permissions p ON rp.permission_id = p.id 
				WHERE rp.template_id = $1
			`, id)
		} else {
			pRows, err = r.db.Query(`
				SELECT p.action 
				FROM app_centric_role_template_permissions rp 
				JOIN permissions p ON rp.permission_id = p.id 
				WHERE rp.template_id = ?
			`, id)
		}
		if err != nil {
			return nil, err
		}

		var perms []string
		for pRows.Next() {
			var p string
			if err := pRows.Scan(&p); err == nil {
				perms = append(perms, p)
			}
		}
		pRows.Close()

		if perms == nil {
			perms = []string{}
		}
		t.Permissions = perms

		templates = append(templates, t)
	}
	if templates == nil {
		templates = []models.Role{}
	}
	return templates, nil
}

// GetUserRolesDetailed returns all Roles (with nested details) assigned or resolved for the user.
func (r *SQLRepository) GetUserRolesDetailed(userID int64) ([]models.Role, error) {
	var rows *sql.Rows
	var err error
	if r.driver == "postgres" {
		rows, err = r.db.Query("SELECT role_id FROM user_tenant_module_roles WHERE user_id = $1", userID)
	} else {
		rows, err = r.db.Query("SELECT role_id FROM user_tenant_module_roles WHERE user_id = ?", userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var directRoleIDs []string
	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err == nil {
			directRoleIDs = append(directRoleIDs, rid)
		}
	}

	if len(directRoleIDs) == 0 {
		return []models.Role{}, nil
	}

	hierarchy, err := r.GetRoleHierarchy()
	if err != nil {
		return nil, err
	}
	resolvedRoleIDs := resolveRoles(directRoleIDs, hierarchy)

	placeholders := make([]string, len(resolvedRoleIDs))
	args := make([]interface{}, len(resolvedRoleIDs))
	for i, id := range resolvedRoleIDs {
		if r.driver == "postgres" {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		} else {
			placeholders[i] = "?"
		}
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, module_id, tenant_id, app_code, name, description, is_system_role, is_active, type, created_at 
		FROM roles 
		WHERE id IN (%s) 
		ORDER BY name ASC`, strings.Join(placeholders, ","))

	rows2, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	var roles []models.Role
	for rows2.Next() {
		var role models.Role
		var tenantID sql.NullString
		var desc sql.NullString
		var roleType sql.NullString
		var createdAtStr string

		err = rows2.Scan(&role.ID, &role.ModuleID, &tenantID, &role.AppCode, &role.Name, &desc, &role.IsSystemRole, &role.IsActive, &roleType, &createdAtStr)
		if err != nil {
			return nil, err
		}
		role.Description = desc.String
		role.Type = roleType.String
		if role.Type == "" {
			role.Type = "Custom"
		}
		if tenantID.Valid {
			tVal := tenantID.String
			role.TenantID = &tVal
		}
		role.CreatedAt, _ = parseTime(createdAtStr)

		// Get mapped permissions
		perms, err := r.getRolePermissions(role.ID)
		if err == nil {
			role.Permissions = perms
		}

		// Get nested roles
		nested, err := r.getNestedRoles(role.ID)
		if err == nil {
			role.NestedRoles = nested
		}

		roles = append(roles, role)
	}

	if roles == nil {
		roles = []models.Role{}
	}

	return roles, nil
}

// GetUserPermissionsDetailed returns all Permission details resolved for the user.
func (r *SQLRepository) GetUserPermissionsDetailed(userID int64) ([]models.Permission, error) {
	var rows *sql.Rows
	var err error
	if r.driver == "postgres" {
		rows, err = r.db.Query("SELECT role_id FROM user_tenant_module_roles WHERE user_id = $1", userID)
	} else {
		rows, err = r.db.Query("SELECT role_id FROM user_tenant_module_roles WHERE user_id = ?", userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var directRoleIDs []string
	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err == nil {
			directRoleIDs = append(directRoleIDs, rid)
		}
	}

	if len(directRoleIDs) == 0 {
		return []models.Permission{}, nil
	}

	hierarchy, err := r.GetRoleHierarchy()
	if err != nil {
		return nil, err
	}
	resolvedRoleIDs := resolveRoles(directRoleIDs, hierarchy)

	placeholders := make([]string, len(resolvedRoleIDs))
	args := make([]interface{}, len(resolvedRoleIDs))
	for i, id := range resolvedRoleIDs {
		if r.driver == "postgres" {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		} else {
			placeholders[i] = "?"
		}
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT DISTINCT p.id, p.module_id, p.action, p.description, p.created_at 
		FROM role_permissions rp 
		JOIN permissions p ON rp.permission_id = p.id 
		WHERE rp.role_id IN (%s) 
		ORDER BY p.action ASC`, strings.Join(placeholders, ","))

	rows2, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	var permissions []models.Permission
	for rows2.Next() {
		var p models.Permission
		var desc sql.NullString
		var createdAtStr string

		err = rows2.Scan(&p.ID, &p.ModuleID, &p.Action, &desc, &createdAtStr)
		if err != nil {
			return nil, err
		}
		p.Description = desc.String
		p.CreatedAt, _ = parseTime(createdAtStr)

		permissions = append(permissions, p)
	}

	if permissions == nil {
		permissions = []models.Permission{}
	}

	return permissions, nil
}

// ListRoleMembers returns usernames of all users holding the given role name.
func (r *SQLRepository) ListRoleMembers(roleName string) ([]string, error) {
	query := `
		SELECT u.username
		FROM user_tenant_module_roles utmr
		JOIN roles r ON utmr.role_id = r.id
		JOIN users u ON utmr.user_id = u.id
		WHERE r.name = ?`
	
	rows, err := r.db.Query(query, roleName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, err
		}
		members = append(members, username)
	}
	if members == nil {
		members = []string{}
	}
	return members, nil
}

