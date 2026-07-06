package handler

import (
	"encoding/json"
	"icarus-auth-ms/internal/models"
	"net/http"
	"strings"
)

type SyncModuleRequest struct {
	Code            string              `json:"code"`
	Name            string              `json:"name"`
	BaseURL         string              `json:"base_url"`
	Permissions     []models.Permission `json:"permissions"`
	DefaultRoles    []models.Role       `json:"default_roles"`
	AppCentricRoles []models.Role       `json:"app_centric_roles"`
	Creator         string              `json:"creator,omitempty"`
}

func (s *HandlerServer) InternalSyncModuleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req SyncModuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Code == "" || req.Name == "" || req.BaseURL == "" {
		s.respondWithError(w, http.StatusBadRequest, "code, name, and base_url are required")
		return
	}

	err := s.Repo.SyncModule(req.Code, req.Name, req.BaseURL, req.Permissions, req.DefaultRoles, req.AppCentricRoles, req.Creator)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to sync module: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func (s *HandlerServer) InternalSyncOnboardApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	moduleID := r.PathValue("id")
	if moduleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "module id is required")
		return
	}

	var req struct {
		AppCode string `json:"app_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AppCode == "" {
		s.respondWithError(w, http.StatusBadRequest, "app_code is required")
		return
	}

	if err := s.Repo.OnboardApplicationToModule(moduleID, req.AppCode); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func (s *HandlerServer) InternalSyncOffboardApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	moduleID := r.PathValue("id")
	appCode := r.PathValue("appCode")
	if moduleID == "" || appCode == "" {
		s.respondWithError(w, http.StatusBadRequest, "module id and app code are required")
		return
	}

	if err := s.Repo.OffboardApplicationFromModule(moduleID, appCode); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

type TokenScopeRequest struct {
	TenantID   string `json:"tenant_id"`
	ModuleCode string `json:"module_code"`
}

func (s *HandlerServer) TokenScopeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		s.respondWithError(w, http.StatusUnauthorized, "missing authorization header")
		return
	}

	authParts := strings.Split(authHeader, " ")
	if len(authParts) != 2 || strings.ToLower(authParts[0]) != "bearer" {
		s.respondWithError(w, http.StatusUnauthorized, "invalid authorization format")
		return
	}

	claims, err := s.TokenMgr.VerifyToken(authParts[1])
	if err != nil {
		s.respondWithError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
		return
	}

	var req TokenScopeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TenantID == "" || req.ModuleCode == "" {
		s.respondWithError(w, http.StatusBadRequest, "tenant_id and module_code are required")
		return
	}

	user, err := s.Repo.GetUserByUsername(claims.Subject)
	if err != nil {
		s.respondWithError(w, http.StatusUnauthorized, "user not found")
		return
	}

	resolvedRoles, resolvedPermissions, err := s.Repo.GetResolvedUserRolesAndPermissions(user.ID, req.TenantID, req.ModuleCode)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to resolve roles: "+err.Error())
		return
	}

	ownedModules, err := s.Repo.GetUserOwnedModules(user.ID)
	if err != nil {
		ownedModules = []string{}
	}

	scopedToken, err := s.TokenMgr.GenerateScopedUserToken(user.Username, req.TenantID, req.ModuleCode, resolvedRoles, resolvedPermissions, user.Groups, ownedModules)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to generate scoped token: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"access_token": scopedToken,
	})
}

// InternalSyncTenantHandler handles tenant synchronization from platform-ms.
func (s *HandlerServer) InternalSyncTenantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req models.Tenant
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" || req.Code == "" || req.Name == "" {
		s.respondWithError(w, http.StatusBadRequest, "id, code, and name are required")
		return
	}

	err := s.Repo.CreateTenant(req.ID, req.Code, req.Name, req.Status)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to sync tenant: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// InternalDeleteTenantHandler handles tenant deletion from platform-ms.
func (s *HandlerServer) InternalDeleteTenantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	err := s.Repo.DeleteTenant(id)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delete tenant: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// InternalDeleteModuleHandler handles module deletion from platform-ms.
func (s *HandlerServer) InternalDeleteModuleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	err := s.Repo.DeleteModule(id)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delete module: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// InternalSyncApplicationHandler handles application synchronization from platform-ms.
func (s *HandlerServer) InternalSyncApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req models.Application
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" || req.Code == "" || req.Name == "" {
		s.respondWithError(w, http.StatusBadRequest, "id, code, and name are required")
		return
	}

	err := s.Repo.CreateApplication(req.ID, req.Code, req.Name, req.Description)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to sync application: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// InternalDeleteApplicationHandler handles application deletion from platform-ms.
func (s *HandlerServer) InternalDeleteApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	err := s.Repo.DeleteApplication(id)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delete application: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// InternalGetModuleRolesAndTemplatesHandler handles retrieving default roles and app-centric role templates for a module.
func (s *HandlerServer) InternalGetModuleRolesAndTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	moduleID := r.PathValue("id")
	if moduleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "module id is required")
		return
	}

	defaultRoles, err := s.Repo.GetModuleDefaultRoles(moduleID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to get default roles: "+err.Error())
		return
	}

	appCentricRoles, err := s.Repo.GetModuleAppCentricRoles(moduleID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to get app centric roles: "+err.Error())
		return
	}

	owners, err := s.Repo.GetModuleOwners(moduleID)
	if err != nil {
		owners = []string{}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"default_roles":     defaultRoles,
		"app_centric_roles": appCentricRoles,
		"owners":            owners,
	})
}

// InternalListRoleMembersHandler GET /internal/roles/{role_name}/members
// Returns usernames of all users holding the given role name.
func (s *HandlerServer) InternalListRoleMembersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	roleName := r.PathValue("role_name")
	if roleName == "" {
		s.respondWithError(w, http.StatusBadRequest, "role_name is required")
		return
	}
	members, err := s.Repo.ListRoleMembers(roleName)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to query role members: "+err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, members)
}

