package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"icarus-admin-ms/internal/models"
	"strings"
	"time"
)

// === APPLICATIONS CRUD ===

func (s *HandlerServer) AdminListApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	apps, err := s.Repo.ListApplications()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, apps)
}

func (s *HandlerServer) AdminCreateApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req models.Application
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
	req.CreatedAt = time.Now()

	// Persist locally
	if err := s.Repo.CreateApplication(&req); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			s.respondWithError(w, http.StatusConflict, "application code already exists")
			return
		}
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Propagate to auth-ms
	authSyncURL := fmt.Sprintf("%s/api/v1/internal/applications/sync", s.AuthMSURL)
	syncPayload, err := json.Marshal(req)
	if err == nil {
		resp, syncErr := http.Post(authSyncURL, "application/json", bytes.NewBuffer(syncPayload))
		if syncErr == nil {
			resp.Body.Close()
		}
	}

	s.respondWithJSON(w, http.StatusCreated, req)
}

func (s *HandlerServer) AdminUpdateApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req models.Application
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.ID = id
	if err := s.Repo.UpdateApplication(&req); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch updated app for sync and response
	apps, err := s.Repo.ListApplications()
	var updatedApp models.Application
	if err == nil {
		for _, a := range apps {
			if a.ID == id {
				updatedApp = a
				break
			}
		}
	}

	if updatedApp.ID != "" {
		authSyncURL := fmt.Sprintf("%s/api/v1/internal/applications/sync", s.AuthMSURL)
		syncPayload, err := json.Marshal(updatedApp)
		if err == nil {
			resp, syncErr := http.Post(authSyncURL, "application/json", bytes.NewBuffer(syncPayload))
			if syncErr == nil {
				resp.Body.Close()
			}
		}
	}

	s.respondWithJSON(w, http.StatusOK, updatedApp)
}

func (s *HandlerServer) AdminDeleteApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := s.Repo.DeleteApplication(id); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Propagate delete to auth-ms
	authDeleteURL := fmt.Sprintf("%s/api/v1/internal/applications/sync/%s", s.AuthMSURL, id)
	req, err := http.NewRequest(http.MethodDelete, authDeleteURL, nil)
	if err == nil {
		client := &http.Client{}
		resp, syncErr := client.Do(req)
		if syncErr == nil {
			resp.Body.Close()
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "application deleted successfully"})
}

// === MODULE APPLICATIONS ONBOARDING ===

func (s *HandlerServer) AdminListModuleApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	moduleID := r.PathValue("id")
	if moduleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "module id is required")
		return
	}

	if !s.canViewModule(r, moduleID) {
		s.respondWithError(w, http.StatusForbidden, "forbidden: only module owner or admin is allowed to view applications for this module")
		return
	}

	apps, err := s.Repo.ListModuleApplications(moduleID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, apps)
}

func (s *HandlerServer) AdminOnboardApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	moduleID := r.PathValue("id")
	if moduleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "module id is required")
		return
	}

	allowed, _ := s.isModuleOwner(r, moduleID)
	if !allowed {
		s.respondWithError(w, http.StatusForbidden, "forbidden: only module owner or admin is allowed to onboard applications to this module")
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

	// Propagate to auth-ms
	authSyncURL := fmt.Sprintf("%s/api/v1/internal/modules/%s/applications/sync", s.AuthMSURL, moduleID)
	syncPayload, err := json.Marshal(req)
	if err == nil {
		resp, syncErr := http.Post(authSyncURL, "application/json", bytes.NewBuffer(syncPayload))
		if syncErr == nil {
			resp.Body.Close()
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "application onboarded successfully"})
}

func (s *HandlerServer) AdminOffboardApplicationHandler(w http.ResponseWriter, r *http.Request) {
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

	allowed, _ := s.isModuleOwner(r, moduleID)
	if !allowed {
		s.respondWithError(w, http.StatusForbidden, "forbidden: only module owner or admin is allowed to offboard applications from this module")
		return
	}

	if err := s.Repo.OffboardApplicationFromModule(moduleID, appCode); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Propagate delete to auth-ms
	authSyncURL := fmt.Sprintf("%s/api/v1/internal/modules/%s/applications/sync/%s", s.AuthMSURL, moduleID, appCode)
	delReq, err := http.NewRequest(http.MethodDelete, authSyncURL, nil)
	if err == nil {
		client := &http.Client{}
		resp, syncErr := client.Do(delReq)
		if syncErr == nil {
			resp.Body.Close()
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "application offboarded successfully"})
}
