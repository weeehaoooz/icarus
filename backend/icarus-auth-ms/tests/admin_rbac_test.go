package tests

import (
	"bytes"
	"encoding/json"
	"icarus-auth-ms/internal/crypto"
	"icarus-auth-ms/internal/handler"
	"icarus-auth-ms/internal/repository"
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
	res, err := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(loginBody))
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
	res, err = http.Post(ts.URL+"/api/v1/register", "application/json", bytes.NewBuffer(regBody))
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
	res, err = http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(bobLoginBody))
	if err != nil {
		t.Fatalf("Bob login failed: %v", err)
	}
	var bobLoginRes map[string]string
	_ = json.NewDecoder(res.Body).Decode(&bobLoginRes)
	bobToken := bobLoginRes["access_token"]

	// 3. Verify Bob is forbidden from admin endpoints
	client := &http.Client{}
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+bobToken)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for non-admin, got %d", res.StatusCode)
	}

	// 4. Verify Admin can list users
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/admin/users", nil)
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
	req, _ = http.NewRequest("POST", ts.URL+"/api/v1/admin/users", bytes.NewBuffer(newUserBody))
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
	req, _ = http.NewRequest("POST", ts.URL+"/api/v1/admin/roles", bytes.NewBuffer(newRoleBody))
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
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/admin/roles", nil)
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

func TestModuleGovernanceRBAC(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mod_gov_test_keys")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbFile := filepath.Join(tempDir, "test_mod_gov.db")
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

	// Register Charlie
	regReq := handler.RegisterRequest{
		Username: "charlie",
		Password: "password123",
	}
	regBody, _ := json.Marshal(regReq)
	_, _ = http.Post(ts.URL+"/api/v1/register", "application/json", bytes.NewBuffer(regBody))

	// Get Charlie ID
	user, err := repo.GetUserByUsername("charlie")
	if err != nil {
		t.Fatalf("Failed to get charlie: %v", err)
	}

	// Sync a module owned by Charlie
	syncReq := handler.SyncModuleRequest{
		Code:    "test-module",
		Name:    "Test Module",
		BaseURL: "http://test-module",
		Creator: "charlie",
	}
	syncBody, _ := json.Marshal(syncReq)
	res, err := http.Post(ts.URL+"/api/v1/internal/modules/sync", "application/json", bytes.NewBuffer(syncBody))
	if err != nil {
		t.Fatalf("Failed to sync module: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected sync status 200, got %d", res.StatusCode)
	}

	// Verify Charlie is now the owner in DB
	owned, err := repo.GetUserOwnedModules(user.ID)
	if err != nil {
		t.Fatalf("Failed to get user owned modules: %v", err)
	}
	if len(owned) != 1 || owned[0] != "test-module" {
		t.Errorf("Expected charlie to own 'test-module', got %v", owned)
	}

	owners, err := repo.GetModuleOwners("test-module")
	if err != nil {
		t.Fatalf("Failed to get module owners: %v", err)
	}
	if len(owners) != 1 || owners[0] != "charlie" {
		t.Errorf("Expected owners of 'test-module' to be ['charlie'], got %v", owners)
	}

	// Verify endpoint returns owners
	endpointRes, err := http.Get(ts.URL + "/api/v1/internal/modules/test-module/roles-and-templates")
	if err != nil {
		t.Fatalf("Failed to call roles-and-templates endpoint: %v", err)
	}
	if endpointRes.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", endpointRes.StatusCode)
	}
	var resData struct {
		Owners []string `json:"owners"`
	}
	_ = json.NewDecoder(endpointRes.Body).Decode(&resData)
	if len(resData.Owners) != 1 || resData.Owners[0] != "charlie" {
		t.Errorf("Expected endpoint to return owners ['charlie'], got %v", resData.Owners)
	}

	// Log in as Charlie and verify owned_modules claim
	loginReq := handler.LoginRequest{
		Username: "charlie",
		Password: "password123",
	}
	loginBody, _ := json.Marshal(loginReq)
	res, err = http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	var loginRes map[string]string
	_ = json.NewDecoder(res.Body).Decode(&loginRes)
	tokenStr := loginRes["access_token"]

	claims, err := tokenMgr.VerifyToken(tokenStr)
	if err != nil {
		t.Fatalf("Token verification failed: %v", err)
	}
	if len(claims.OwnedModules) != 1 || claims.OwnedModules[0] != "test-module" {
		t.Errorf("Expected token to contain owned_modules ['test-module'], got %v", claims.OwnedModules)
	}
}

func TestAdminClientMultipleRoles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "client_rbac_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbFile := filepath.Join(tempDir, "test_client_rbac.db")
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

	// Authenticate as Admin
	loginReq := handler.LoginRequest{
		Username: "admin",
		Password: "admin123",
	}
	loginBody, _ := json.Marshal(loginReq)
	res, err := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("Admin login failed: %v", err)
	}
	var loginRes map[string]string
	_ = json.NewDecoder(res.Body).Decode(&loginRes)
	adminToken := loginRes["access_token"]

	// Create client with multiple roles
	createClientReq := handler.AdminCreateClientRequest{
		ClientID:  "test-client-1",
		PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0...\n-----END PUBLIC KEY-----",
		Roles:     []string{"admin", "user"},
	}
	body, _ := json.Marshal(createClientReq)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/admin/clients", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for client creation with multiple roles, got %d", res.StatusCode)
	}

	// Verify client roles in GetClient
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/admin/clients/test-client-1", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", res.StatusCode)
	}
	var clientData struct {
		ClientID string   `json:"client_id"`
		Roles    []string `json:"roles"`
	}
	_ = json.NewDecoder(res.Body).Decode(&clientData)
	if len(clientData.Roles) != 2 {
		t.Errorf("Expected 2 roles for client, got %d (%v)", len(clientData.Roles), clientData.Roles)
	}
}

