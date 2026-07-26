package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"icarus-auth-ms/internal/crypto"
	"icarus-auth-ms/internal/handler"
	"icarus-auth-ms/internal/repository"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAccountStatusManagement(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "account_status_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbFile := filepath.Join(tempDir, "test_status.db")
	privKey, pubKey, err := crypto.LoadOrGenerateKeys(tempDir)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeys failed: %v", err)
	}

	dbConn, err := repository.InitDB("sqlite", dbFile)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer dbConn.Close()

	repo := repository.NewSQLRepository(dbConn, "sqlite")
	tokenMgr := crypto.NewTokenManager(privKey, pubKey, "icarus-auth-ms")
	server := handler.NewHandlerServer(repo, tokenMgr)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Seed admin user
	if err := repo.SeedDefaultRBAC(); err != nil {
		t.Fatalf("Failed to seed admin user: %v", err)
	}

	// 1. Admin Login to get admin token
	adminToken := getAdminToken(t, ts)

	// 2. Register test user
	regReq := map[string]string{
		"username":   "statususer",
		"password":   "password123",
		"email":      "statususer@example.com",
		"first_name": "Status",
		"last_name":  "Test",
	}
	body, _ := json.Marshal(regReq)
	resp, err := http.Post(ts.URL+"/register", "application/json", bytes.NewBuffer(body))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("Failed to register user: %v, status: %d", err, resp.StatusCode)
	}

	userObj, err := repo.GetUserByUsername("statususer")
	if err != nil {
		t.Fatalf("Failed to fetch registered user: %v", err)
	}
	if !userObj.IsActive {
		t.Fatalf("Expected new user to be active by default")
	}

	// 3. User Login (should succeed when active)
	loginReq := map[string]string{
		"username": "statususer",
		"password": "password123",
	}
	body, _ = json.Marshal(loginReq)
	resp, err = http.Post(ts.URL+"/login", "application/json", bytes.NewBuffer(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed for active user: %v", err)
	}

	var loginResp map[string]string
	json.NewDecoder(resp.Body).Decode(&loginResp)
	accessToken := loginResp["access_token"]
	refreshToken := loginResp["refresh_token"]

	// 4. Disable user via Admin API
	statusReq := map[string]bool{"is_active": false}
	body, _ = json.Marshal(statusReq)
	req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/admin/users/%d/status", ts.URL, userObj.ID), bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to disable user: %v, status: %d", err, resp.StatusCode)
	}

	// 5. Attempt login with disabled account (should fail 401)
	body, _ = json.Marshal(loginReq)
	resp, err = http.Post(ts.URL+"/login", "application/json", bytes.NewBuffer(body))
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for disabled user login, got %d", resp.StatusCode)
	}

	// 6. Attempt token refresh with revoked refresh token (should fail 401)
	refReq := map[string]string{"refresh_token": refreshToken}
	body, _ = json.Marshal(refReq)
	resp, err = http.Post(ts.URL+"/refresh", "application/json", bytes.NewBuffer(body))
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for refresh token of disabled user, got %d", resp.StatusCode)
	}

	// 7. Access protected /me endpoint with old access token (UserRequired middleware should block disabled user)
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized for disabled user /me request, got %d", resp.StatusCode)
	}

	// 8. Re-enable user via Admin API
	statusReq = map[string]bool{"is_active": true}
	body, _ = json.Marshal(statusReq)
	req, _ = http.NewRequest(http.MethodPut, fmt.Sprintf("%s/admin/users/%d/status", ts.URL, userObj.ID), bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to re-enable user: %v, status: %d", err, resp.StatusCode)
	}

	// 9. Login again after re-enabling (should succeed)
	body, _ = json.Marshal(loginReq)
	resp, err = http.Post(ts.URL+"/login", "application/json", bytes.NewBuffer(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed after re-enabling user: %v, status: %d", err, resp.StatusCode)
	}
}

func getAdminToken(t *testing.T, ts *httptest.Server) string {
	loginReq := map[string]string{
		"username": "admin",
		"password": "admin123",
	}
	body, _ := json.Marshal(loginReq)
	resp, err := http.Post(ts.URL+"/login", "application/json", bytes.NewBuffer(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to login as admin: %v, status: %d", err, resp.StatusCode)
	}

	var loginResp map[string]string
	json.NewDecoder(resp.Body).Decode(&loginResp)
	return loginResp["access_token"]
}
