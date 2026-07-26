package handler

import (
	"encoding/json"
	"icarus-auth-ms/internal/models"
	"icarus-auth-ms/internal/securitylog"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// AdminRequired middleware ensures the request has a valid admin JWT.
func (s *HandlerServer) AdminRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.SecLogger.LogEvent(r.Context(), securitylog.Event{
				EventType:      securitylog.DomainAccessControl,
				Action:         "ACCESS_DENIED",
				Severity:       securitylog.SeverityWarn,
				ActorIP:        securitylog.GetClientIP(r),
				UserAgent:      r.UserAgent(),
				TargetResource: r.URL.Path,
				Status:         securitylog.StatusFailure,
				Details:        map[string]interface{}{"reason": "missing authorization header"},
			})
			s.respondWithError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			s.SecLogger.LogEvent(r.Context(), securitylog.Event{
				EventType:      securitylog.DomainAccessControl,
				Action:         "ACCESS_DENIED",
				Severity:       securitylog.SeverityWarn,
				ActorIP:        securitylog.GetClientIP(r),
				UserAgent:      r.UserAgent(),
				TargetResource: r.URL.Path,
				Status:         securitylog.StatusFailure,
				Details:        map[string]interface{}{"reason": "invalid authorization format"},
			})
			s.respondWithError(w, http.StatusUnauthorized, "invalid authorization format")
			return
		}

		tokenStr := parts[1]
		claims, err := s.TokenMgr.VerifyToken(tokenStr)
		if err != nil {
			s.SecLogger.LogEvent(r.Context(), securitylog.Event{
				EventType:      securitylog.DomainAccessControl,
				Action:         "ACCESS_DENIED",
				Severity:       securitylog.SeverityWarn,
				ActorIP:        securitylog.GetClientIP(r),
				UserAgent:      r.UserAgent(),
				TargetResource: r.URL.Path,
				Status:         securitylog.StatusFailure,
				Details:        map[string]interface{}{"reason": "invalid token: " + err.Error()},
			})
			s.respondWithError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}

		user, err := s.Repo.GetUserByUsername(claims.Subject)
		if err == nil && !user.IsActive {
			s.SecLogger.LogEvent(r.Context(), securitylog.Event{
				EventType:      securitylog.DomainAccessControl,
				Action:         "ACCESS_DENIED",
				Severity:       securitylog.SeverityWarn,
				Actor:          claims.Subject,
				ActorIP:        securitylog.GetClientIP(r),
				UserAgent:      r.UserAgent(),
				TargetResource: r.URL.Path,
				Status:         securitylog.StatusFailure,
				Details:        map[string]interface{}{"reason": "account is disabled"},
			})
			s.respondWithError(w, http.StatusUnauthorized, "account is disabled")
			return
		}

		r.Header.Set("X-Username", claims.Subject)

		isAdmin := false
		for _, role := range claims.Roles {
			if role == "admin" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			s.SecLogger.LogEvent(r.Context(), securitylog.Event{
				EventType:      securitylog.DomainAccessControl,
				Action:         "ACCESS_DENIED",
				Severity:       securitylog.SeverityWarn,
				Actor:          claims.Subject,
				ActorIP:        securitylog.GetClientIP(r),
				UserAgent:      r.UserAgent(),
				TargetResource: r.URL.Path,
				Status:         securitylog.StatusFailure,
				Details:        map[string]interface{}{"reason": "admin role required"},
			})
			s.respondWithError(w, http.StatusForbidden, "forbidden: admin role required")
			return
		}

		next(w, r)
	}
}

// === USERS MANAGEMENT ===

type AdminCreateUserRequest struct {
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	Email     string   `json:"email"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Roles     []string `json:"roles"`
	IsActive  *bool    `json:"is_active,omitempty"`
}

type AdminUpdateUserRequest struct {
	Username  string   `json:"username"`
	Password  string   `json:"password,omitempty"` // empty means keep current password
	Email     string   `json:"email"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Roles     []string `json:"roles"`
	IsActive  *bool    `json:"is_active,omitempty"`
}

type AdminUpdateUserStatusRequest struct {
	IsActive bool `json:"is_active"`
}

func (s *HandlerServer) AdminListUsersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	users, err := s.Repo.ListUsers()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to list users: "+err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, users)
}

func (s *HandlerServer) AdminCreateUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req AdminCreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		s.respondWithError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to process password")
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	userID, err := s.Repo.CreateUserWithStatus(req.Username, req.Email, req.FirstName, req.LastName, string(hashedBytes), isActive)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505") {
			s.respondWithError(w, http.StatusConflict, "username already exists")
			return
		}
		s.respondWithError(w, http.StatusInternalServerError, "failed to create user: "+err.Error())
		return
	}

	if len(req.Roles) > 0 {
		if err := s.Repo.AssignUserRoles(userID, req.Roles); err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "user created but failed to assign roles: "+err.Error())
			return
		}
	}

	s.respondWithJSON(w, http.StatusCreated, map[string]interface{}{
		"message":   "user created successfully",
		"id":        userID,
		"is_active": isActive,
	})
}

func (s *HandlerServer) AdminGetUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	user, err := s.Repo.GetUserByID(id)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "user not found")
		return
	}

	roles, err := s.Repo.GetUserRoles(id)
	if err != nil {
		roles = []string{}
	}
	user.Roles = roles

	s.respondWithJSON(w, http.StatusOK, user)
}

func (s *HandlerServer) AdminUpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	existingUser, err := s.Repo.GetUserByID(id)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "user not found")
		return
	}

	var req AdminUpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" {
		s.respondWithError(w, http.StatusBadRequest, "username is required")
		return
	}

	var passwordHash string
	if req.Password != "" {
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "failed to process password")
			return
		}
		passwordHash = string(hashedBytes)
	}

	isActive := existingUser.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	err = s.Repo.UpdateUserWithStatus(id, req.Username, req.Email, req.FirstName, req.LastName, passwordHash, isActive)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to update user: "+err.Error())
		return
	}

	err = s.Repo.AssignUserRoles(id, req.Roles)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "user updated but failed to assign roles: "+err.Error())
		return
	}

	if !isActive {
		_ = s.Repo.DeleteUserRefreshTokens(id)
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "user updated successfully",
		"is_active": isActive,
	})
}

func (s *HandlerServer) AdminUpdateUserStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req AdminUpdateUserStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.Repo.UpdateUserStatus(id, req.IsActive); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to update user status: "+err.Error())
		return
	}

	if !req.IsActive {
		_ = s.Repo.DeleteUserRefreshTokens(id)
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainUserMgmt,
		Action:         "UPDATE_USER_STATUS",
		Severity:       securitylog.SeverityInfo,
		Actor:          r.Header.Get("X-Username"),
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: strconv.FormatInt(id, 10),
		Status:         securitylog.StatusSuccess,
		Details:        map[string]interface{}{"is_active": req.IsActive},
	})

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "user status updated successfully",
		"id":        id,
		"is_active": req.IsActive,
	})
}

func (s *HandlerServer) AdminDeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	err = s.Repo.DeleteUser(id)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delete user: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "user deleted successfully"})
}

type AdminAssignUserRoleRequest struct {
	TenantID string `json:"tenant_id"`
	ModuleID string `json:"module_id"`
	RoleID   string `json:"role_id"`
}

func (s *HandlerServer) AdminAssignUserRoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	user, err := s.Repo.GetUserByID(id)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "user not found")
		return
	}

	var req AdminAssignUserRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RoleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "role_id is required")
		return
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = "system-tenant"
	}

	moduleID := req.ModuleID
	if moduleID == "" {
		moduleID = "icarus-auth-ms"
	}

	err = s.Repo.AssignUserTenantModuleRole(user.ID, tenantID, moduleID, req.RoleID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to assign role: "+err.Error())
		return
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainRoleMgmt,
		Action:         "ASSIGN_USER_ROLE",
		Severity:       securitylog.SeverityInfo,
		Actor:          r.Header.Get("X-Username"),
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: strconv.FormatInt(user.ID, 10),
		Status:         securitylog.StatusSuccess,
		Details:        map[string]interface{}{"tenant_id": tenantID, "module_id": moduleID, "role_id": req.RoleID},
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "role assigned successfully"})
}

func (s *HandlerServer) AdminRevokeUserRoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	user, err := s.Repo.GetUserByID(id)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "user not found")
		return
	}

	roleID := r.PathValue("roleId")
	if roleID == "" {
		roleID = r.URL.Query().Get("role_id")
	}
	if roleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "roleId is required")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	moduleID := r.URL.Query().Get("module_id")

	err = s.Repo.RevokeUserRoleSpecific(user.ID, tenantID, moduleID, roleID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to revoke role: "+err.Error())
		return
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainRoleMgmt,
		Action:         "REVOKE_USER_ROLE",
		Severity:       securitylog.SeverityInfo,
		Actor:          r.Header.Get("X-Username"),
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: strconv.FormatInt(user.ID, 10),
		Status:         securitylog.StatusSuccess,
		Details:        map[string]interface{}{"tenant_id": tenantID, "module_id": moduleID, "role_id": roleID},
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "role revoked successfully"})
}


// === CLIENTS MANAGEMENT ===

type AdminCreateClientRequest struct {
	ClientID  string   `json:"client_id"`
	PublicKey string   `json:"public_key"`
	Roles     []string `json:"roles"`
}

type AdminUpdateClientRequest struct {
	PublicKey string   `json:"public_key"`
	Roles     []string `json:"roles"`
}

func (s *HandlerServer) AdminListClientsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	clients, err := s.Repo.ListClients()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to list clients: "+err.Error())
		return
	}
	if clients == nil {
		clients = []models.Client{}
	}
	s.respondWithJSON(w, http.StatusOK, clients)
}

func (s *HandlerServer) AdminCreateClientHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req AdminCreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ClientID == "" || req.PublicKey == "" {
		s.respondWithError(w, http.StatusBadRequest, "client_id and public_key are required")
		return
	}

	err := s.Repo.CreateClient(req.ClientID, req.PublicKey)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to create client: "+err.Error())
		return
	}

	if len(req.Roles) > 0 {
		err = s.Repo.AssignClientRoles(req.ClientID, req.Roles)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "client created but failed to assign roles: "+err.Error())
			return
		}
	}

	s.respondWithJSON(w, http.StatusCreated, map[string]string{
		"message":   "client registered successfully",
		"client_id": req.ClientID,
	})
}

func (s *HandlerServer) AdminGetClientHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	clientID := r.PathValue("id")
	if clientID == "" {
		s.respondWithError(w, http.StatusBadRequest, "missing client id")
		return
	}

	client, err := s.Repo.GetClientByID(clientID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "client not found")
		return
	}

	roles, err := s.Repo.GetClientRoles(clientID)
	if err != nil {
		roles = []string{}
	}
	client.Roles = roles

	s.respondWithJSON(w, http.StatusOK, client)
}

func (s *HandlerServer) AdminUpdateClientHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	clientID := r.PathValue("id")
	if clientID == "" {
		s.respondWithError(w, http.StatusBadRequest, "missing client id")
		return
	}

	var req AdminUpdateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PublicKey == "" {
		s.respondWithError(w, http.StatusBadRequest, "public_key is required")
		return
	}

	err := s.Repo.CreateClient(clientID, req.PublicKey)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to update client public key: "+err.Error())
		return
	}

	err = s.Repo.AssignClientRoles(clientID, req.Roles)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "client updated but failed to assign roles: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "client updated successfully"})
}

func (s *HandlerServer) AdminDeleteClientHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	clientID := r.PathValue("id")
	if clientID == "" {
		s.respondWithError(w, http.StatusBadRequest, "missing client id")
		return
	}

	err := s.Repo.DeleteClient(clientID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delete client: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "client deleted successfully"})
}

// === ROLES & PERMISSIONS ===

type AdminCreateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	NestedRoles []string `json:"nested_roles"`
	Type        string   `json:"type"` // "LDAP" or "Custom"
	ModuleID    string   `json:"module_id"`
	AppCode     string   `json:"app_code"`
	IsActive    *bool    `json:"is_active"`
}

type AdminUpdateRoleRequest struct {
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	NestedRoles []string `json:"nested_roles"`
	Type        string   `json:"type"` // "LDAP" or "Custom"
	ModuleID    string   `json:"module_id"`
	AppCode     string   `json:"app_code"`
	IsActive    *bool    `json:"is_active"`
}

func (s *HandlerServer) AdminListRolesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	qParams := r.URL.Query()
	search := qParams.Get("q")
	limitStr := qParams.Get("limit")
	offsetStr := qParams.Get("offset")

	var limit, offset int
	var err error
	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			s.respondWithError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}
	}
	if offsetStr != "" {
		offset, err = strconv.Atoi(offsetStr)
		if err != nil {
			s.respondWithError(w, http.StatusBadRequest, "invalid offset parameter")
			return
		}
	}

	var roles []models.Role
	if limit > 0 || search != "" {
		roles, err = s.Repo.ListRolesPaged(search, limit, offset)
	} else {
		roles, err = s.Repo.ListRoles()
	}

	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to list roles: "+err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, roles)
}

func (s *HandlerServer) AdminCreateRoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req AdminCreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		s.respondWithError(w, http.StatusBadRequest, "role name is required")
		return
	}

	if req.ModuleID == "" {
		req.ModuleID = "icarus-auth-ms"
	}

	err := s.Repo.CreateRole(req.ModuleID, req.AppCode, req.Name, req.Description, req.Type)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to create role: "+err.Error())
		return
	}

	roleID := req.ModuleID + ":" + req.Name
	if req.AppCode != "" {
		roleID = req.ModuleID + ":" + req.AppCode + ":" + req.Name
	}

	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	err = s.Repo.UpdateRole(roleID, req.Description, req.Type, active, req.Permissions, req.NestedRoles)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "role created but failed to map permissions or nested roles: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusCreated, map[string]string{"message": "role created successfully"})
}

func (s *HandlerServer) AdminUpdateRoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "missing role id")
		return
	}

	var req AdminUpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	err := s.Repo.UpdateRole(id, req.Description, req.Type, active, req.Permissions, req.NestedRoles)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to update role: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "role updated successfully"})
}

func (s *HandlerServer) AdminDeleteRoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "missing role id")
		return
	}

	if id == "icarus-auth-ms:admin" || id == "admin" {
		s.respondWithError(w, http.StatusBadRequest, "the admin role is system protected and cannot be deleted")
		return
	}

	err := s.Repo.DeleteRole(id)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delete role: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "role deleted successfully"})
}

func (s *HandlerServer) AdminListPermissionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	perms, err := s.Repo.ListPermissions()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to list permissions: "+err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, perms)
}

func (s *HandlerServer) AdminListApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	apps, err := s.Repo.ListApplications()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to list applications: "+err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, apps)
}
