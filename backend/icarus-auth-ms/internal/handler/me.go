package handler

import (
	"encoding/json"
	"fmt"
	"icarus-auth-ms/internal/securitylog"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// UserRequired middleware ensures the request has a valid JWT token.
func (s *HandlerServer) UserRequired(next http.HandlerFunc) http.HandlerFunc {
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
		s.SecLogger.LogEvent(r.Context(), securitylog.Event{
			EventType:      securitylog.DomainAuth,
			Action:         "CHANGE_PASSWORD",
			Severity:       securitylog.SeverityWarn,
			Actor:          username,
			ActorIP:        securitylog.GetClientIP(r),
			UserAgent:      r.UserAgent(),
			TargetResource: fmt.Sprintf("user:%d", user.ID),
			Status:         securitylog.StatusFailure,
			Details:        map[string]interface{}{"reason": "incorrect old password"},
		})
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

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainAuth,
		Action:         "CHANGE_PASSWORD",
		Severity:       securitylog.SeverityInfo,
		Actor:          username,
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: fmt.Sprintf("user:%d", user.ID),
		Status:         securitylog.StatusSuccess,
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{"message": "password changed successfully"})
}

// MeRolesHandler returns the list of roles assigned to the currently authenticated user.
func (s *HandlerServer) MeRolesHandler(w http.ResponseWriter, r *http.Request) {
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

	roles, err := s.Repo.GetUserRolesDetailed(user.ID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to get roles: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, roles)
}

// MePermissionsHandler returns the list of resolved permissions for the currently authenticated user.
func (s *HandlerServer) MePermissionsHandler(w http.ResponseWriter, r *http.Request) {
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

	permissions, err := s.Repo.GetUserPermissionsDetailed(user.ID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to get permissions: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, permissions)
}

// UserSummary is a stripped-down user representation safe to expose to all authenticated users.
type UserSummary struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// UserDirectoryHandler returns a list of all users with only safe, non-sensitive fields.
// Accessible by any authenticated user (UserRequired).
func (s *HandlerServer) UserDirectoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	users, err := s.Repo.ListUsers()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to list users: "+err.Error())
		return
	}

	summaries := make([]UserSummary, 0, len(users))
	for _, u := range users {
		summaries = append(summaries, UserSummary{
			ID:        u.ID,
			Username:  u.Username,
			FirstName: u.FirstName,
			LastName:  u.LastName,
		})
	}

	s.respondWithJSON(w, http.StatusOK, summaries)
}

// UserRolesPublicHandler returns the detailed role list for a specific user by ID.
// Only role metadata is returned — no personal user data. Accessible by any authenticated user.
func (s *HandlerServer) UserRolesPublicHandler(w http.ResponseWriter, r *http.Request) {
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

	roles, err := s.Repo.GetUserRolesDetailed(id)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to get user roles: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, roles)
}

