package tests

import (
	"icarus-auth-ms/internal/crypto"
	"icarus-auth-ms/internal/handler"
	"icarus-auth-ms/internal/repository"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAdminRBACFlow(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "rbac_test_keys")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbFile := filepath.Join(tempDir, "test_rbac.db")
	defer os.Remove(dbFile)

	privKey, pubKey, err := crypto.LoadOrGenerateKeys(tempDir)
	if err != nil {
		t.Fatalf("Failed to load/generate keys: %v", err)
	}

	dbConn, err := repository.InitDB("sqlite", dbFile)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer dbConn.Close()

	repo := repository.NewSQLRepository(dbConn, "sqlite")
	if err := repo.SeedDefaultRBAC(); err != nil {
		t.Fatalf("Failed to seed default RBAC data: %v", err)
	}

	tokenMgr := crypto.NewTokenManager(privKey, pubKey, "icarus-auth-ms")
	server := handler.NewHandlerServer(repo, tokenMgr)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 1. Authenticate as seeded Admin
	loginReq := handler.LoginRequest{
		Username: "admin",
		Password: "admin123",
	}
	loginBody, _ := json.Marshal(loginReq)
	res, err := http.Post(ts.URL+"/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("Admin login failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected login success, got status %d", res.StatusCode)
	}

	var loginRes map[string]string
	_ = json.NewDecoder(res.Body).Decode(&loginRes)
	adminToken := loginRes["access_token"]

	// 2. Authenticate as regular user (non-admin)
	regReq := handler.RegisterRequest{
		Username: "bob",
		Password: "password123",
	}
	regBody, _ := json.Marshal(regReq)
	res, err = http.Post(ts.URL+"/register", "application/json", bytes.NewBuffer(regBody))
	if err != nil {
		t.Fatalf("Register bob failed: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created, got %d", res.StatusCode)
	}

	bobLoginReq := handler.LoginRequest{
		Username: "bob",
		Password: "password123",
	}
	bobLoginBody, _ := json.Marshal(bobLoginReq)
	res, err = http.Post(ts.URL+"/login", "application/json", bytes.NewBuffer(bobLoginBody))
	if err != nil {
		t.Fatalf("Bob login failed: %v", err)
	}
	var bobLoginRes map[string]string
	_ = json.NewDecoder(res.Body).Decode(&bobLoginRes)
	bobToken := bobLoginRes["access_token"]

	// 3. Verify Bob is forbidden from admin endpoints
	client := &http.Client{}
	req, _ := http.NewRequest("GET", ts.URL+"/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+bobToken)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for non-admin, got %d", res.StatusCode)
	}

	// 4. Verify Admin can list users
	req, _ = http.NewRequest("GET", ts.URL+"/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK for admin, got %d", res.StatusCode)
	}

	var users []map[string]interface{}
	_ = json.NewDecoder(res.Body).Decode(&users)
	if len(users) != 2 { // admin and bob
		t.Errorf("Expected 2 users, got %d", len(users))
	}

	// 5. Verify Admin can create a user with roles
	newUserReq := handler.AdminCreateUserRequest{
		Username:  "charlie",
		Password:  "charlie123",
		Email:     "charlie@example.com",
		FirstName: "Charlie",
		LastName:  "Brown",
		Roles:     []string{"admin"},
	}
	newUserBody, _ := json.Marshal(newUserReq)
	req, _ = http.NewRequest("POST", ts.URL+"/admin/users", bytes.NewBuffer(newUserBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("Create user failed: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Errorf("Expected 201 Created, got %d", res.StatusCode)
	}

	// Check that Charlie exists and has the admin role
	dbCharlie, err := repo.GetUserByUsername("charlie")
	if err != nil {
		t.Fatalf("Failed to fetch charlie: %v", err)
	}
	roles, err := repo.GetUserRoles(dbCharlie.ID)
	if err != nil || len(roles) != 1 || roles[0] != "admin" {
		t.Errorf("Expected roles to be ['admin'], got %v", roles)
	}

	// 6. Verify Admin can create a role and map permissions
	newRoleReq := handler.AdminCreateRoleRequest{
		Name:        "editor",
		Description: "Can edit posts",
		Permissions: []string{"manage:users"},
	}
	newRoleBody, _ := json.Marshal(newRoleReq)
	req, _ = http.NewRequest("POST", ts.URL+"/admin/roles", bytes.NewBuffer(newRoleBody))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("Create role failed: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Errorf("Expected 201 Created, got %d", res.StatusCode)
	}

	// List roles and check
	req, _ = http.NewRequest("GET", ts.URL+"/admin/roles", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("List roles failed: %v", err)
	}
	var rolesRes []map[string]interface{}
	_ = json.NewDecoder(res.Body).Decode(&rolesRes)
	foundEditor := false
	for _, r := range rolesRes {
		if r["name"] == "editor" {
			foundEditor = true
			break
		}
	}
	if !foundEditor {
		t.Error("Expected to find editor role in listed roles")
	}
}
