package tests

import (
	"icarus-auth-ms/internal/crypto"
	"icarus-auth-ms/internal/handler"
	"icarus-auth-ms/internal/ratelimit"
	"icarus-auth-ms/internal/repository"

	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthFlowModular(t *testing.T) {
	// 1. Setup temporary directories/files for testing
	tempDir, err := os.MkdirTemp("", "auth_test_keys_modular")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbFile := filepath.Join(tempDir, "test_auth_modular.db")
	defer os.Remove(dbFile)

	// 2. Initialize RSA Keys
	privKey, pubKey, err := crypto.LoadOrGenerateKeys(tempDir)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeys failed: %v", err)
	}

	// 3. Initialize Database
	dbConn, err := repository.InitDB("sqlite", dbFile)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer dbConn.Close()

	repo := repository.NewSQLRepository(dbConn, "sqlite")
	tokenMgr := crypto.NewTokenManager(privKey, pubKey, "icarus-auth-ms")

	// 4. Initialize Handler Server
	server := handler.NewHandlerServer(repo, tokenMgr)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// ==========================================
	// TEST CASE 1: User Registration
	// ==========================================
	regReq := handler.RegisterRequest{
		Username:  "alice",
		Password:  "supersecretpassword",
		Email:     "alice@example.com",
		FirstName: "Alice",
		LastName:  "Smith",
	}
	regBody, _ := json.Marshal(regReq)

	res, err := http.Post(ts.URL+"/api/v1/register", "application/json", bytes.NewBuffer(regBody))
	if err != nil {
		t.Fatalf("POST /register failed: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201 Created, got %d", res.StatusCode)
	}

	// Verify database contains the registered fields
	dbUser, err := repo.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}
	if dbUser.Email != "alice@example.com" {
		t.Errorf("Expected email to be alice@example.com, got %s", dbUser.Email)
	}
	if dbUser.FirstName != "Alice" {
		t.Errorf("Expected FirstName to be Alice, got %s", dbUser.FirstName)
	}
	if dbUser.LastName != "Smith" {
		t.Errorf("Expected LastName to be Smith, got %s", dbUser.LastName)
	}

	// Test conflict/duplicate register
	resConf, _ := http.Post(ts.URL+"/api/v1/register", "application/json", bytes.NewBuffer(regBody))
	if resConf.StatusCode != http.StatusConflict {
		t.Errorf("Expected status 409 Conflict for duplicate registration, got %d", resConf.StatusCode)
	}

	// ==========================================
	// TEST CASE 2: User Login
	// ==========================================
	loginReq := handler.LoginRequest{
		Username: "alice",
		Password: "supersecretpassword",
	}
	loginBody, _ := json.Marshal(loginReq)

	res, err = http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("POST /login failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d", res.StatusCode)
	}

	var loginRes map[string]string
	err = json.NewDecoder(res.Body).Decode(&loginRes)
	if err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}

	accessToken := loginRes["access_token"]
	refreshToken := loginRes["refresh_token"]

	if accessToken == "" || refreshToken == "" {
		t.Fatal("Login did not return access_token and refresh_token")
	}

	// ==========================================
	// TEST CASE 3: Access Token Verification
	// ==========================================
	client := &http.Client{}
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/verify", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /verify failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", res.StatusCode)
	}

	var claims crypto.CustomClaims
	_ = json.NewDecoder(res.Body).Decode(&claims)
	if claims.Subject != "alice" || claims.Type != "user" {
		t.Errorf("Token claims mismatch. Expected sub=alice, type=user. Got sub=%s, type=%s", claims.Subject, claims.Type)
	}

	// ==========================================
	// TEST CASE 4: Token Refresh & Rotation
	// ==========================================
	refReq := handler.RefreshRequest{
		RefreshToken: refreshToken,
	}
	refBody, _ := json.Marshal(refReq)

	res, err = http.Post(ts.URL+"/api/v1/refresh", "application/json", bytes.NewBuffer(refBody))
	if err != nil {
		t.Fatalf("POST /refresh failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d", res.StatusCode)
	}

	var refRes map[string]string
	_ = json.NewDecoder(res.Body).Decode(&refRes)

	newAccessToken := refRes["access_token"]
	newRefreshToken := refRes["refresh_token"]

	if newAccessToken == "" || newRefreshToken == "" {
		t.Fatal("Refresh did not return new tokens")
	}

	// Try to reuse the old refresh token (should fail since it was rotated)
	resReuse, err := http.Post(ts.URL+"/api/v1/refresh", "application/json", bytes.NewBuffer(refBody))
	if err != nil {
		t.Fatalf("POST /refresh failed: %v", err)
	}
	if resReuse.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected old refresh token reuse to fail with 401 Unauthorized, got %d", resReuse.StatusCode)
	}

	// ==========================================
	// TEST CASE 4b: Logout & Token Revocation
	// ==========================================
	// 4b-1. Revoke single refresh token via POST /logout
	logoutReq := handler.LogoutRequest{
		RefreshToken: newRefreshToken,
	}
	logoutBody, _ := json.Marshal(logoutReq)

	resLogout, err := http.Post(ts.URL+"/api/v1/logout", "application/json", bytes.NewBuffer(logoutBody))
	if err != nil {
		t.Fatalf("POST /logout failed: %v", err)
	}
	if resLogout.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK for POST /logout, got %d", resLogout.StatusCode)
	}

	// Verify the revoked refresh token can no longer be refreshed
	refReqRevoked := handler.RefreshRequest{
		RefreshToken: newRefreshToken,
	}
	refBodyRevoked, _ := json.Marshal(refReqRevoked)
	resPostLogoutRefresh, err := http.Post(ts.URL+"/api/v1/refresh", "application/json", bytes.NewBuffer(refBodyRevoked))
	if err != nil {
		t.Fatalf("POST /refresh failed: %v", err)
	}
	if resPostLogoutRefresh.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected refreshed token to return 401 Unauthorized post-logout, got %d", resPostLogoutRefresh.StatusCode)
	}

	// 4b-2. Multi-device / All-devices Logout
	// Login twice to generate two active refresh tokens
	resDev1, _ := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(loginBody))
	var loginDev1 map[string]string
	_ = json.NewDecoder(resDev1.Body).Decode(&loginDev1)
	tokenDev1 := loginDev1["refresh_token"]
	accTokenDev1 := loginDev1["access_token"]

	resDev2, _ := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(loginBody))
	var loginDev2 map[string]string
	_ = json.NewDecoder(resDev2.Body).Decode(&loginDev2)
	tokenDev2 := loginDev2["refresh_token"]

	if tokenDev1 == "" || tokenDev2 == "" {
		t.Fatal("Failed to generate multi-device refresh tokens")
	}

	// Logout from all devices using Bearer auth and all_devices: true
	allDevReq := handler.LogoutRequest{
		AllDevices: true,
	}
	allDevBody, _ := json.Marshal(allDevReq)
	reqAllDev, _ := http.NewRequest("POST", ts.URL+"/api/v1/logout", bytes.NewBuffer(allDevBody))
	reqAllDev.Header.Set("Authorization", "Bearer "+accTokenDev1)
	reqAllDev.Header.Set("Content-Type", "application/json")

	resAllDev, err := client.Do(reqAllDev)
	if err != nil {
		t.Fatalf("POST /logout (all_devices) failed: %v", err)
	}
	if resAllDev.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK for all_devices logout, got %d", resAllDev.StatusCode)
	}

	// Verify both tokens are now invalid
	refDev1Body, _ := json.Marshal(handler.RefreshRequest{RefreshToken: tokenDev1})
	resRefDev1, _ := http.Post(ts.URL+"/api/v1/refresh", "application/json", bytes.NewBuffer(refDev1Body))
	if resRefDev1.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected tokenDev1 refresh to fail after all_devices logout, got %d", resRefDev1.StatusCode)
	}

	refDev2Body, _ := json.Marshal(handler.RefreshRequest{RefreshToken: tokenDev2})
	resRefDev2, _ := http.Post(ts.URL+"/api/v1/refresh", "application/json", bytes.NewBuffer(refDev2Body))
	if resRefDev2.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected tokenDev2 refresh to fail after all_devices logout, got %d", resRefDev2.StatusCode)
	}


	// ==========================================
	// TEST CASE 5: Public Keys & JWKS Endpoint
	// ==========================================
	res, err = http.Get(ts.URL + "/api/v1/certs")
	if err != nil {
		t.Fatalf("GET /certs failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK for JWKS, got %d", res.StatusCode)
	}

	var jwks map[string]interface{}
	_ = json.NewDecoder(res.Body).Decode(&jwks)
	keys, ok := jwks["keys"].([]interface{})
	if !ok || len(keys) == 0 {
		t.Error("Certs did not return valid JWKS structure")
	}

	// Check format=pem
	resPem, err := http.Get(ts.URL + "/api/v1/certs?format=pem")
	if err != nil {
		t.Fatalf("GET /certs?format=pem failed: %v", err)
	}
	if resPem.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK for PEM format, got %d", resPem.StatusCode)
	}

	// ==========================================
	// TEST CASE 6: Client Key-Pair Authorization
	// ==========================================
	// 6a. Generate Client Keypair
	clientPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate client RSA: %v", err)
	}

	clientPubBytes := x509.MarshalPKCS1PublicKey(&clientPrivKey.PublicKey)
	clientPubPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: clientPubBytes,
	})

	// 6b. Register Client
	clientReq := handler.ClientRegisterRequest{
		ClientID:  "test-client-99",
		PublicKey: string(clientPubPem),
	}
	clientBody, _ := json.Marshal(clientReq)

	res, err = http.Post(ts.URL+"/api/v1/client/register", "application/json", bytes.NewBuffer(clientBody))
	if err != nil {
		t.Fatalf("POST /client/register failed: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("Expected status 201 Created for client registration, got %d", res.StatusCode)
	}

	// 6c. Construct and Sign Client Assertion JWT
	assertionClaims := jwt.RegisteredClaims{
		Issuer:    "test-client-99",
		Subject:   "test-client-99",
		Audience:  jwt.ClaimStrings{"icarus-auth-ms"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	assertionToken := jwt.NewWithClaims(jwt.SigningMethodRS256, assertionClaims)
	assertionStr, err := assertionToken.SignedString(clientPrivKey)
	if err != nil {
		t.Fatalf("Failed to sign client assertion: %v", err)
	}

	// 6d. Request Client Access Token
	clientTokReq := handler.ClientTokenRequest{
		ClientAssertion: assertionStr,
	}
	clientTokBody, _ := json.Marshal(clientTokReq)

	res, err = http.Post(ts.URL+"/api/v1/client/token", "application/json", bytes.NewBuffer(clientTokBody))
	if err != nil {
		t.Fatalf("POST /client/token failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected client /token response 200 OK, got %d", res.StatusCode)
	}

	var clientTokRes map[string]string
	_ = json.NewDecoder(res.Body).Decode(&clientTokRes)

	clientAccessToken := clientTokRes["access_token"]
	if clientAccessToken == "" {
		t.Fatal("Client /token response missing access_token")
	}

	// 6e. Verify Client Access Token
	req, _ = http.NewRequest("GET", ts.URL+"/api/v1/verify", nil)
	req.Header.Set("Authorization", "Bearer "+clientAccessToken)

	res, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /verify client token failed: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected client token verification 200 OK, got %d", res.StatusCode)
	}

	var clientClaims crypto.CustomClaims
	_ = json.NewDecoder(res.Body).Decode(&clientClaims)
	if clientClaims.Subject != "test-client-99" || clientClaims.Type != "client" {
		t.Errorf("Client token claims mismatch. Expected sub=test-client-99, type=client. Got sub=%s, type=%s", clientClaims.Subject, clientClaims.Type)
	}
}

func TestBruteForceProtection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auth_test_bruteforce")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbFile := filepath.Join(tempDir, "test_auth_bf.db")
	defer os.Remove(dbFile)

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

	// Register a target user for lockout testing
	regReq := handler.RegisterRequest{
		Username: "victim_user",
		Password: "CorrectPassword123!",
		Email:    "victim@example.com",
	}
	regBody, _ := json.Marshal(regReq)
	_, _ = http.Post(ts.URL+"/api/v1/register", "application/json", bytes.NewBuffer(regBody))

	// 1. Test Account Lockout after 5 failed password attempts
	badLoginReq := handler.LoginRequest{
		Username: "victim_user",
		Password: "WrongPassword!",
	}
	badLoginBody, _ := json.Marshal(badLoginReq)

	for i := 1; i <= 5; i++ {
		res, err := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(badLoginBody))
		if err != nil {
			t.Fatalf("Failed request %d: %v", i, err)
		}
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("Attempt %d: expected status 401 Unauthorized, got %d", i, res.StatusCode)
		}
	}

	// 6th attempt should be blocked by Account Lockout (429 Too Many Requests)
	resLocked, err := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(badLoginBody))
	if err != nil {
		t.Fatalf("6th attempt request failed: %v", err)
	}
	if resLocked.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected 6th attempt to return status 429 Too Many Requests (Account Locked), got %d", resLocked.StatusCode)
	}

	// Even with correct password, account is locked
	goodLoginReq := handler.LoginRequest{
		Username: "victim_user",
		Password: "CorrectPassword123!",
	}
	goodLoginBody, _ := json.Marshal(goodLoginReq)
	resGoodLocked, _ := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(goodLoginBody))
	if resGoodLocked.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected status 429 for locked user even with correct password, got %d", resGoodLocked.StatusCode)
	}

	// Reset attempt tracker and IP limiter for IP rate limit test
	server.AttemptTracker.RecordSuccess("victim_user")
	server.IPLimiter = ratelimit.NewIPLimiter(10, 1*time.Minute)

	// 2. Test IP Rate Limiting

	// Default IP limiter allows 10 requests per minute
	for i := 1; i <= 10; i++ {
		res, _ := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(goodLoginBody))
		if i <= 10 && res.StatusCode != http.StatusOK {
			t.Errorf("Attempt %d: expected 200 OK before hitting IP rate limit, got %d", i, res.StatusCode)
		}
	}

	// 11th request from same IP should be blocked by IP Rate Limiting (429)
	resIpExceeded, _ := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(goodLoginBody))
	if resIpExceeded.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected 11th request from same IP to return 429 Too Many Requests, got %d", resIpExceeded.StatusCode)
	}
}

func TestAdminContextualUserRoleAssignment(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auth_test_admin_roles")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbFile := filepath.Join(tempDir, "test_admin_roles.db")
	defer os.Remove(dbFile)

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
	if err := repo.SeedDefaultRBAC(); err != nil {
		t.Fatalf("SeedDefaultRBAC failed: %v", err)
	}

	tokenMgr := crypto.NewTokenManager(privKey, pubKey, "icarus-auth-ms")
	server := handler.NewHandlerServer(repo, tokenMgr)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := &http.Client{}

	// Login as admin
	adminLoginReq := handler.LoginRequest{Username: "admin", Password: "admin123"}
	adminLoginBody, _ := json.Marshal(adminLoginReq)
	resAdminLogin, err := http.Post(ts.URL+"/api/v1/login", "application/json", bytes.NewBuffer(adminLoginBody))
	if err != nil || resAdminLogin.StatusCode != http.StatusOK {
		t.Fatalf("Admin login failed: %v, code %d", err, resAdminLogin.StatusCode)
	}
	var adminRes map[string]string
	_ = json.NewDecoder(resAdminLogin.Body).Decode(&adminRes)
	adminToken := adminRes["access_token"]

	// Seed tenant-1, module-1, and role module-1:editor
	_ = repo.CreateTenant("tenant-1", "t1", "Tenant 1", "active")
	_ = repo.SyncModule("module-1", "Module 1", "http://localhost", nil, nil, nil, "")
	_ = repo.CreateRole("module-1", "", "editor", "Editor role", "Custom")

	// Create test user Dave
	daveID, err := repo.CreateUser("dave", "dave@example.com", "Dave", "User", "pass123")
	if err != nil {
		t.Fatalf("Failed to create user Dave: %v", err)
	}

	// 1. Assign role to Dave for tenant-1 and module-1 via POST /admin/users/{id}/roles
	assignReq := handler.AdminAssignUserRoleRequest{
		TenantID: "tenant-1",
		ModuleID: "module-1",
		RoleID:   "module-1:editor",
	}

	assignBody, _ := json.Marshal(assignReq)
	reqAssign, _ := http.NewRequest("POST", ts.URL+"/api/v1/admin/users/"+strconv.FormatInt(daveID, 10)+"/roles", bytes.NewBuffer(assignBody))
	reqAssign.Header.Set("Authorization", "Bearer "+adminToken)
	reqAssign.Header.Set("Content-Type", "application/json")

	resAssign, err := client.Do(reqAssign)
	if err != nil {
		t.Fatalf("POST /admin/users/{id}/roles failed: %v", err)
	}
	if resAssign.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK for role assignment, got %d", resAssign.StatusCode)
	}

	// Verify Dave's resolved roles for tenant-1 and module-1
	roles, perms, err := repo.GetResolvedUserRolesAndPermissions(daveID, "tenant-1", "module-1")
	if err != nil {
		t.Fatalf("GetResolvedUserRolesAndPermissions failed: %v", err)
	}
	_ = perms
	hasEditorRole := false
	for _, r := range roles {
		if r == "module-1:editor" || r == "editor" {
			hasEditorRole = true
			break
		}
	}
	if !hasEditorRole {
		t.Errorf("Expected Dave to have module-1:editor role, got roles: %v", roles)
	}

	// 2. Revoke role from Dave via DELETE /admin/users/{id}/roles/{roleId}?tenant_id=tenant-1&module_id=module-1
	reqRevoke, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/admin/users/"+strconv.FormatInt(daveID, 10)+"/roles/module-1:editor?tenant_id=tenant-1&module_id=module-1", nil)
	reqRevoke.Header.Set("Authorization", "Bearer "+adminToken)

	resRevoke, err := client.Do(reqRevoke)
	if err != nil {
		t.Fatalf("DELETE /admin/users/{id}/roles/{roleId} failed: %v", err)
	}
	if resRevoke.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK for role revocation, got %d", resRevoke.StatusCode)
	}

	// Verify Dave's roles after revocation
	rolesAfter, _, _ := repo.GetResolvedUserRolesAndPermissions(daveID, "tenant-1", "module-1")
	for _, r := range rolesAfter {
		if r == "module-1:editor" || r == "editor" {
			t.Errorf("Expected module-1:editor role to be revoked, but found in roles: %v", rolesAfter)
		}
	}
}


