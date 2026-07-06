package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"icarus-admin-ms/internal/models"
	"net/http"
)

type OnboardModuleRequest struct {
	Code            string              `json:"code"`
	Name            string              `json:"name"`
	BaseURL         string              `json:"base_url"`
	Permissions     []models.Permission `json:"permissions"`
	DefaultRoles    []models.Role       `json:"default_roles"`
	AppCentricRoles []models.Role       `json:"app_centric_roles"`
}

func (s *HandlerServer) OnboardModuleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req OnboardModuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Code == "" || req.Name == "" || req.BaseURL == "" {
		s.respondWithError(w, http.StatusBadRequest, "code, name, and base_url are required")
		return
	}

	// 1. Map to models
	module := models.Module{
		ID:              req.Code, // use code as ID
		Code:            req.Code,
		Name:            req.Name,
		BaseURL:         req.BaseURL,
		AppCentricRoles: req.AppCentricRoles,
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

	// 2. Persist in platform-ms local DB
	allRoles := append(req.DefaultRoles, req.AppCentricRoles...)
	err := s.Repo.RegisterModule(&module, permissions, allRoles)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to persist module locally: "+err.Error())
		return
	}

	// 3. Propagate to auth-ms via internal HTTP sync endpoint
	authSyncURL := fmt.Sprintf("%s/internal/modules/sync", s.AuthMSURL)
	syncPayload, err := json.Marshal(req)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to marshal sync request: "+err.Error())
		return
	}

	resp, err := http.Post(authSyncURL, "application/json", bytes.NewBuffer(syncPayload))
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to synchronize with auth-ms: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&errorResp)
		errMsg := "unknown error"
		if errorResp != nil && errorResp["error"] != "" {
			errMsg = errorResp["error"]
		}
		s.respondWithError(w, resp.StatusCode, "auth-ms synchronization failed: "+errMsg)
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "success",
		"module_id": module.ID,
		"message":   "Module successfully onboarded and synchronized",
	})
}
