package handler

import (
	"icarus-auth-ms/internal/ldap"
	"icarus-auth-ms/internal/models"
	"encoding/json"
	"net/http"
)

const passwordPlaceholder = "******"

func (s *HandlerServer) AdminGetLDAPConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cfg, err := s.Repo.GetLDAPConfig()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to get LDAP config: "+err.Error())
		return
	}

	// Mask the bind password for security
	if cfg.BindPassword != "" {
		cfg.BindPassword = passwordPlaceholder
	}

	s.respondWithJSON(w, http.StatusOK, cfg)
}

func (s *HandlerServer) AdminUpdateLDAPConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req models.LDAPConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Handle password placeholder
	if req.BindPassword == passwordPlaceholder {
		existing, err := s.Repo.GetLDAPConfig()
		if err == nil {
			req.BindPassword = existing.BindPassword
		} else {
			req.BindPassword = ""
		}
	}

	err := s.Repo.UpdateLDAPConfig(&req)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to update LDAP config: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "LDAP configuration updated successfully"})
}

func (s *HandlerServer) AdminTestLDAPConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req models.LDAPConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Handle password placeholder
	if req.BindPassword == passwordPlaceholder {
		existing, err := s.Repo.GetLDAPConfig()
		if err == nil {
			req.BindPassword = existing.BindPassword
		} else {
			req.BindPassword = ""
		}
	}

	err := ldap.TestLDAPConnection(&req)
	if err != nil {
		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "LDAP connection test passed successfully",
	})
}
