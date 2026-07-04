package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"icarus-admin-ms/internal/crypto"
	"icarus-admin-ms/internal/models"
	"strings"
	"time"
)

type contextKey string
const claimsContextKey contextKey = "claims"

// AdminRequired middleware ensures the request has a valid admin or module owner JWT.
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
		claims, err := s.Verifier.VerifyToken(tokenStr)
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

		isModuleOwner := len(claims.OwnedModules) > 0

		if !isAdmin && !isModuleOwner {
			s.respondWithError(w, http.StatusForbidden, "forbidden: admin or module owner role required")
			return
		}

		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func (s *HandlerServer) isSuperAdmin(r *http.Request) bool {
	claims, ok := r.Context().Value(claimsContextKey).(*crypto.CustomClaims)
	if !ok || claims == nil {
		return false
	}
	for _, role := range claims.Roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

func (s *HandlerServer) isModuleOwner(r *http.Request, moduleID string) (bool, *crypto.CustomClaims) {
	claims, ok := r.Context().Value(claimsContextKey).(*crypto.CustomClaims)
	if !ok || claims == nil {
		return false, nil
	}

	// Super-admins are allowed to do anything (per user's feedback)
	for _, role := range claims.Roles {
		if role == "admin" {
			return true, claims
		}
	}

	for _, m := range claims.OwnedModules {
		if m == moduleID {
			return true, claims
		}
	}
	return false, claims
}

func (s *HandlerServer) canViewModule(r *http.Request, moduleID string) bool {
	claims, ok := r.Context().Value(claimsContextKey).(*crypto.CustomClaims)
	if !ok || claims == nil {
		return false
	}

	// Super-admins can view all modules
	for _, role := range claims.Roles {
		if role == "admin" {
			return true
		}
	}

	for _, m := range claims.OwnedModules {
		if m == moduleID {
			return true
		}
	}
	return false
}

// === TENANTS CRUD ===

func (s *HandlerServer) AdminListTenantsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	tenants, err := s.Repo.ListTenants()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, tenants)
}

func (s *HandlerServer) AdminCreateTenantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req models.Tenant
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Code == "" || req.Name == "" {
		s.respondWithError(w, http.StatusBadRequest, "code and name are required")
		return
	}

	if req.ID == "" {
		req.ID = req.Code
	}
	if req.Status == "" {
		req.Status = "active"
	}
	req.CreatedAt = time.Now()

	// Persist locally
	if err := s.Repo.CreateTenant(&req); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			s.respondWithError(w, http.StatusConflict, "tenant code already exists")
			return
		}
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Propagate to auth-ms
	authSyncURL := fmt.Sprintf("%s/internal/tenants/sync", s.AuthMSURL)
	syncPayload, err := json.Marshal(req)
	if err == nil {
		resp, syncErr := http.Post(authSyncURL, "application/json", bytes.NewBuffer(syncPayload))
		if syncErr == nil {
			resp.Body.Close()
		}
	}

	s.respondWithJSON(w, http.StatusCreated, req)
}

func (s *HandlerServer) AdminUpdateTenantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req models.Tenant
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.ID = id
	if err := s.Repo.UpdateTenant(&req); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch updated tenant for sync and response
	tenants, err := s.Repo.ListTenants()
	var updatedTenant models.Tenant
	if err == nil {
		for _, t := range tenants {
			if t.ID == id {
				updatedTenant = t
				break
			}
		}
	}

	if updatedTenant.ID != "" {
		authSyncURL := fmt.Sprintf("%s/internal/tenants/sync", s.AuthMSURL)
		syncPayload, err := json.Marshal(updatedTenant)
		if err == nil {
			resp, syncErr := http.Post(authSyncURL, "application/json", bytes.NewBuffer(syncPayload))
			if syncErr == nil {
				resp.Body.Close()
			}
		}
	}

	s.respondWithJSON(w, http.StatusOK, updatedTenant)
}

func (s *HandlerServer) AdminDeleteTenantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := s.Repo.DeleteTenant(id); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Propagate delete to auth-ms
	authDeleteURL := fmt.Sprintf("%s/internal/tenants/sync/%s", s.AuthMSURL, id)
	req, err := http.NewRequest(http.MethodDelete, authDeleteURL, nil)
	if err == nil {
		client := &http.Client{}
		resp, syncErr := client.Do(req)
		if syncErr == nil {
			resp.Body.Close()
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "tenant deleted successfully"})
}

// === MODULES CRUD ===

func (s *HandlerServer) AdminListModulesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	modules, err := s.Repo.ListModules()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter modules if the user is not a Super-admin
	if !s.isSuperAdmin(r) {
		claims, ok := r.Context().Value(claimsContextKey).(*crypto.CustomClaims)
		var owned []string
		if ok && claims != nil {
			owned = claims.OwnedModules
		}
		
		ownedSet := make(map[string]bool)
		for _, m := range owned {
			ownedSet[m] = true
		}

		var filtered []models.Module
		for _, m := range modules {
			if ownedSet[m.ID] || ownedSet[m.Code] {
				filtered = append(filtered, m)
			}
		}
		modules = filtered
	}

	if modules == nil {
		modules = []models.Module{}
	}

	s.respondWithJSON(w, http.StatusOK, modules)
}

type AdminModuleSaveRequest struct {
	Code            string              `json:"code"`
	Name            string              `json:"name"`
	BaseURL         string              `json:"base_url"`
	IsActive        bool                `json:"is_active"`
	Permissions     []models.Permission `json:"permissions"`
	DefaultRoles    []models.Role       `json:"default_roles"`
	AppCentricRoles []models.Role       `json:"app_centric_roles"`
	Creator         string              `json:"creator,omitempty"`
}

func (s *HandlerServer) AdminCreateModuleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !s.isSuperAdmin(r) {
		s.respondWithError(w, http.StatusForbidden, "forbidden: only super-admin is allowed to register new modules")
		return
	}

	var req AdminModuleSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Code == "" || req.Name == "" || req.BaseURL == "" {
		s.respondWithError(w, http.StatusBadRequest, "code, name, and base_url are required")
		return
	}

	claims, ok := r.Context().Value(claimsContextKey).(*crypto.CustomClaims)
	if ok && claims != nil {
		req.Creator = claims.Subject
	}

	module := models.Module{
		ID:        req.Code,
		Code:      req.Code,
		Name:      req.Name,
		BaseURL:   req.BaseURL,
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	permissions := make([]models.Permission, len(req.Permissions))
	for i, p := range req.Permissions {
		permissions[i] = models.Permission{
			ID:          req.Code + ":" + p.Action,
			ModuleID:    req.Code,
			Action:      p.Action,
			PathPattern: p.PathPattern,
			Method:      p.Method,
			Description: p.Description,
		}
	}

	if err := s.Repo.RegisterModule(&module, permissions); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Propagate to auth-ms via SyncModule endpoint
	authSyncURL := fmt.Sprintf("%s/internal/modules/sync", s.AuthMSURL)
	syncPayload, err := json.Marshal(req)
	if err == nil {
		resp, syncErr := http.Post(authSyncURL, "application/json", bytes.NewBuffer(syncPayload))
		if syncErr == nil {
			resp.Body.Close()
		}
	}

	s.respondWithJSON(w, http.StatusCreated, module)
}

func (s *HandlerServer) AdminUpdateModuleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	allowed, _ := s.isModuleOwner(r, id)
	if !allowed {
		s.respondWithError(w, http.StatusForbidden, "forbidden: only module owner or admin is allowed to update this module")
		return
	}

	var req AdminModuleSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	module := models.Module{
		ID:       id,
		Code:     req.Code,
		Name:     req.Name,
		BaseURL:  req.BaseURL,
		IsActive: req.IsActive,
	}

	permissions := make([]models.Permission, len(req.Permissions))
	for i, p := range req.Permissions {
		permissions[i] = models.Permission{
			ID:          req.Code + ":" + p.Action,
			ModuleID:    req.Code,
			Action:      p.Action,
			PathPattern: p.PathPattern,
			Method:      p.Method,
			Description: p.Description,
		}
	}

	if err := s.Repo.RegisterModule(&module, permissions); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync to auth-ms
	authSyncURL := fmt.Sprintf("%s/internal/modules/sync", s.AuthMSURL)
	syncPayload, err := json.Marshal(req)
	if err == nil {
		resp, syncErr := http.Post(authSyncURL, "application/json", bytes.NewBuffer(syncPayload))
		if syncErr == nil {
			resp.Body.Close()
		}
	}

	s.respondWithJSON(w, http.StatusOK, module)
}

func (s *HandlerServer) AdminGetModuleManifestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	if !s.canViewModule(r, id) {
		s.respondWithError(w, http.StatusForbidden, "forbidden: only module owner or admin is allowed to view this module's manifest")
		return
	}

	module, err := s.Repo.GetModuleByCode(id)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "module not found: "+err.Error())
		return
	}

	permissions, err := s.Repo.GetPermissionsByModuleID(module.ID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to get permissions: "+err.Error())
		return
	}

	authSyncURL := fmt.Sprintf("%s/internal/modules/%s/roles-and-templates", s.AuthMSURL, module.Code)
	resp, err := http.Get(authSyncURL)
	var defaultRoles []models.Role
	var appCentricRoles []models.Role
	owners := []string{}

	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var authData struct {
				DefaultRoles    []models.Role `json:"default_roles"`
				AppCentricRoles []models.Role `json:"app_centric_roles"`
				Owners          []string      `json:"owners"`
			}
			if jsonErr := json.NewDecoder(resp.Body).Decode(&authData); jsonErr == nil {
				defaultRoles = authData.DefaultRoles
				appCentricRoles = authData.AppCentricRoles
				if authData.Owners != nil {
					owners = authData.Owners
				}
			}
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"code":              module.Code,
		"name":              module.Name,
		"base_url":          module.BaseURL,
		"is_active":         module.IsActive,
		"permissions":       permissions,
		"default_roles":     defaultRoles,
		"app_centric_roles": appCentricRoles,
		"owners":           owners,
	})
}

func (s *HandlerServer) AdminDeleteModuleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	allowed, _ := s.isModuleOwner(r, id)
	if !allowed {
		s.respondWithError(w, http.StatusForbidden, "forbidden: only module owner or admin is allowed to delete this module")
		return
	}

	if err := s.Repo.DeleteModule(id); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Propagate to auth-ms
	authDeleteURL := fmt.Sprintf("%s/internal/modules/sync/%s", s.AuthMSURL, id)
	req, err := http.NewRequest(http.MethodDelete, authDeleteURL, nil)
	if err == nil {
		client := &http.Client{}
		resp, syncErr := client.Do(req)
		if syncErr == nil {
			resp.Body.Close()
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "module deleted successfully"})
}
