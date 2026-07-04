package handler

import (
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
)

// UserRequired middleware ensures the request has a valid JWT token.
func (s *HandlerServer) UserRequired(next http.HandlerFunc) http.HandlerFunc {
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

		// Inject username into request headers for downstream handler consumption
		r.Header.Set("X-Username", claims.Subject)
		next(w, r)
	}
}

// MeGetHandler returns details of the currently authenticated user.
func (s *HandlerServer) MeGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	username := r.Header.Get("X-Username")
	user, err := s.Repo.GetUserByUsername(username)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "user not found")
		return
	}

	// Fetch roles as well (all assigned role names across all tenants/modules)
	roles, err := s.Repo.GetUserRoles(user.ID)
	if err == nil {
		user.Roles = roles
	}

	s.respondWithJSON(w, http.StatusOK, user)
}

type UpdateProfileRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// MeUpdateHandler updates first name, last name, and email of the currently authenticated user.
func (s *HandlerServer) MeUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	username := r.Header.Get("X-Username")
	user, err := s.Repo.GetUserByUsername(username)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "user not found")
		return
	}

	if req.Email == "" {
		s.respondWithError(w, http.StatusBadRequest, "email is required")
		return
	}

	err = s.Repo.UpdateUser(user.ID, username, req.Email, req.FirstName, req.LastName, "")
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to update profile: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "profile updated successfully"})
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// MeChangePasswordHandler changes the password of the currently authenticated user.
func (s *HandlerServer) MeChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		s.respondWithError(w, http.StatusBadRequest, "old password and new password are required")
		return
	}

	username := r.Header.Get("X-Username")
	user, err := s.Repo.GetUserByUsername(username)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "user not found")
		return
	}

	// Compare old password with hashed password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		s.respondWithError(w, http.StatusUnauthorized, "incorrect old password")
		return
	}

	// Hash new password
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to process password")
		return
	}

	err = s.Repo.UpdateUser(user.ID, username, user.Email, user.FirstName, user.LastName, string(hashedBytes))
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to change password: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "password changed successfully"})
}
