package tests

import (
	"icarus-auth-ms/internal/repository"
	"os"
	"path/filepath"
	"testing"
)

func TestHierarchicalRBAC(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "repo_rbac_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbFile := filepath.Join(tempDir, "test_rbac_hierarchy.db")
	dbConn, err := repository.InitDB("sqlite", dbFile)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer dbConn.Close()

	repo := repository.NewSQLRepository(dbConn, "sqlite")

	// 1. Create permissions
	_, _ = dbConn.Exec("INSERT INTO permissions (name, description) VALUES ('perm:a', 'Perm A')")
	_, _ = dbConn.Exec("INSERT INTO permissions (name, description) VALUES ('perm:b', 'Perm B')")
	_, _ = dbConn.Exec("INSERT INTO permissions (name, description) VALUES ('perm:c', 'Perm C')")

	// 2. Create roles with different types
	err = repo.CreateRole("icarus-auth-ms", "", "role_c", "Role C", "Custom")
	if err != nil {
		t.Fatalf("Failed to create role_c: %v", err)
	}
	err = repo.CreateRole("icarus-auth-ms", "", "role_b", "Role B", "Custom")
	if err != nil {
		t.Fatalf("Failed to create role_b: %v", err)
	}
	err = repo.CreateRole("icarus-auth-ms", "", "role_a", "Role A", "LDAP")
	if err != nil {
		t.Fatalf("Failed to create role_a: %v", err)
	}

	// 3. Map permissions to roles
	// role_c gets perm:c
	err = repo.UpdateRole("role_c", "Role C", "Custom", true, []string{"perm:c"}, nil)
	if err != nil {
		t.Fatalf("Failed to update role_c permissions: %v", err)
	}
	// role_b gets perm:b and nests role_c
	err = repo.UpdateRole("role_b", "Role B", "Custom", true, []string{"perm:b"}, []string{"role_c"})
	if err != nil {
		t.Fatalf("Failed to update role_b: %v", err)
	}
	// role_a gets perm:a and nests role_b
	err = repo.UpdateRole("role_a", "Role A", "LDAP", true, []string{"perm:a"}, []string{"role_b"})
	if err != nil {
		t.Fatalf("Failed to update role_a: %v", err)
	}

	// 4. Verify ListRoles returns correct types and nested roles
	roles, err := repo.ListRoles()
	if err != nil {
		t.Fatalf("Failed to list roles: %v", err)
	}

	var foundA, foundB, foundC bool
	for _, r := range roles {
		switch r.Name {
		case "role_a":
			foundA = true
			if r.Type != "LDAP" {
				t.Errorf("Expected role_a to be LDAP, got %s", r.Type)
			}
			if len(r.NestedRoles) != 1 || r.NestedRoles[0] != "role_b" {
				t.Errorf("Expected role_a nested roles to be ['role_b'], got %v", r.NestedRoles)
			}
		case "role_b":
			foundB = true
			if r.Type != "Custom" {
				t.Errorf("Expected role_b to be Custom, got %s", r.Type)
			}
			if len(r.NestedRoles) != 1 || r.NestedRoles[0] != "role_c" {
				t.Errorf("Expected role_b nested roles to be ['role_c'], got %v", r.NestedRoles)
			}
		case "role_c":
			foundC = true
			if r.Type != "Custom" {
				t.Errorf("Expected role_c to be Custom, got %s", r.Type)
			}
			if len(r.NestedRoles) != 0 {
				t.Errorf("Expected role_c nested roles to be empty, got %v", r.NestedRoles)
			}
		}
	}
	if !foundA || !foundB || !foundC {
		t.Errorf("Failed to find all created roles: found A:%t, B:%t, C:%t", foundA, foundB, foundC)
	}

	// 5. Create user and assign role_a
	res, err := dbConn.Exec("INSERT INTO users (username, password_hash) VALUES ('testuser', 'hash')")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	userID, _ := res.LastInsertId()

	err = repo.AssignUserRoles(userID, []string{"role_a"})
	if err != nil {
		t.Fatalf("Failed to assign role to user: %v", err)
	}

	// 6. Test hierarchical resolution
	resolvedRoles, resolvedPermissions, err := repo.GetResolvedUserRolesAndPermissions(userID, "system-tenant", "icarus-auth-ms")
	if err != nil {
		t.Fatalf("Failed to resolve user roles and permissions: %v", err)
	}

	// Verify roles: role_a, role_b, role_c should be resolved
	rolesMap := make(map[string]bool)
	for _, r := range resolvedRoles {
		rolesMap[r] = true
	}
	if !rolesMap["role_a"] || !rolesMap["role_b"] || !rolesMap["role_c"] {
		t.Errorf("Expected resolved roles to contain role_a, role_b, role_c, got %v", resolvedRoles)
	}

	// Verify permissions: perm:a, perm:b, perm:c should be resolved
	permsMap := make(map[string]bool)
	for _, p := range resolvedPermissions {
		permsMap[p] = true
	}
	if !permsMap["perm:a"] || !permsMap["perm:b"] || !permsMap["perm:c"] {
		t.Errorf("Expected resolved permissions to contain perm:a, perm:b, perm:c, got %v", resolvedPermissions)
	}

	// 7. Test recursive nesting cycle detection
	// role_c nests role_a -> cycle (role_a -> role_b -> role_c -> role_a)
	err = repo.UpdateRole("role_c", "Role C", "Custom", true, []string{"perm:c"}, []string{"role_a"})
	if err == nil {
		t.Error("Expected error when introducing cycle (role_c nesting role_a), got nil")
	} else if err.Error() != "invalid role hierarchy: recursive nesting detected" {
		t.Errorf("Expected cycle detection error, got: %v", err)
	}
}
