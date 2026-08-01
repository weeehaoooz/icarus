package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"icarus-admin-ms/internal/models"
	"net/http"
	"strings"
	"time"
)

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
	authSyncURL := fmt.Sprintf("%s/api/v1/internal/tenants/sync", s.AuthMSURL)
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
		authSyncURL := fmt.Sprintf("%s/api/v1/internal/tenants/sync", s.AuthMSURL)
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
	authDeleteURL := fmt.Sprintf("%s/api/v1/internal/tenants/sync/%s", s.AuthMSURL, id)
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
