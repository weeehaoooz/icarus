package handler

import (
	"icarus-auth-ms/internal/models"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strconv"
	"strings"
)

// AdminRequired middleware ensures the request has a valid admin JWT.
func (s *HandlerServer) AdminRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.respondWithError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			s.respondWithError(w, http.StatusUnauthorized, "invalid authorization format")
			return
		}

		tokenStr := parts[1]
		claims, err := s.TokenMgr.VerifyToken(tokenStr)
		if err != nil {
			s.respondWithError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}

		isAdmin := false
		for _, role := range claims.Roles {
			if role == "admin" {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
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
}

type AdminUpdateUserRequest struct {
	Username  string   `json:"username"`
	Password  string   `json:"password,omitempty"` // empty means keep current password
	Email     string   `json:"email"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Roles     []string `json:"roles"`
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

	userID, err := s.Repo.CreateUser(req.Username, req.Email, req.FirstName, req.LastName, string(hashedBytes))
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
		"message": "user created successfully",
		"id":      userID,
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

	err = s.Repo.UpdateUser(id, req.Username, req.Email, req.FirstName, req.LastName, passwordHash)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to update user: "+err.Error())
		return
	}

	err = s.Repo.AssignUserRoles(id, req.Roles)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "user updated but failed to assign roles: "+err.Error())
		return
	}


	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "user updated successfully"})
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
