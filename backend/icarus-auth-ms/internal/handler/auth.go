package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"icarus-auth-ms/internal/ldap"
	"icarus-auth-ms/internal/models"
	"icarus-auth-ms/internal/securitylog"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type RegisterRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func generateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// RegisterHandler registers a new user.
func (s *HandlerServer) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req RegisterRequest
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
			s.SecLogger.LogEvent(r.Context(), securitylog.Event{
				EventType: securitylog.DomainUserMgmt,
				Action:    "REGISTER_USER",
				Severity:  securitylog.SeverityWarn,
				Actor:     req.Username,
				ActorIP:   securitylog.GetClientIP(r),
				UserAgent: r.UserAgent(),
				Status:    securitylog.StatusFailure,
				Details:   map[string]interface{}{"reason": "username already exists"},
			})
			s.respondWithError(w, http.StatusConflict, "username already exists")
			return
		}
		s.respondWithError(w, http.StatusInternalServerError, "failed to register user: "+err.Error())
		return
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainUserMgmt,
		Action:         "REGISTER_USER",
		Severity:       securitylog.SeverityInfo,
		Actor:          req.Username,
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: fmt.Sprintf("user:%d", userID),
		Status:         securitylog.StatusSuccess,
	})

	s.respondWithJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "user registered successfully",
		"user_id": userID,
	})
}

// LoginHandler authenticates a user, issuing access token (5m) and refresh token (24h).
func (s *HandlerServer) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var user *models.User
	var err error
	ldapAuthenticated := false
	authMethod := "local"

	// Check if LDAP is enabled
	ldapCfg, ldapErr := s.Repo.GetLDAPConfig()
	if ldapErr == nil && ldapCfg != nil && ldapCfg.Enabled {
		log.Printf("LDAP enabled. Attempting authentication for user: %s", req.Username)
		ldapUser, authErr := ldap.AuthenticateLDAP(ldapCfg, req.Username, req.Password)
		if authErr == nil {
			log.Printf("LDAP authentication successful for user: %s. Fetching/provisioning local user...", req.Username)
			ldapAuthenticated = true
			authMethod = "ldap"

			// Get or provision local user
			localUser, getErr := s.Repo.GetUserByUsername(req.Username)
			if getErr != nil {
				// Provision user (JIT)
				randomPass, _ := generateSecureToken(32)
				hashedBytes, cryptErr := bcrypt.GenerateFromPassword([]byte(randomPass), bcrypt.DefaultCost)
				if cryptErr != nil {
					s.respondWithError(w, http.StatusInternalServerError, "failed to hash random password for LDAP user")
					return
				}

				userID, createErr := s.Repo.CreateUser(ldapUser.Username, ldapUser.Email, ldapUser.FirstName, ldapUser.LastName, string(hashedBytes))
				if createErr != nil {
					s.respondWithError(w, http.StatusInternalServerError, "failed to provision LDAP user: "+createErr.Error())
					return
				}

				localUser = &models.User{
					ID:        userID,
					Username:  ldapUser.Username,
					Email:     ldapUser.Email,
					FirstName: ldapUser.FirstName,
					LastName:  ldapUser.LastName,
				}
				log.Printf("JIT provisioned local user record for LDAP user: %s (ID: %d)", req.Username, userID)
			} else {
				// Update existing local user's fields if changed
				if localUser.Email != ldapUser.Email || localUser.FirstName != ldapUser.FirstName || localUser.LastName != ldapUser.LastName {
					_ = s.Repo.UpdateUser(localUser.ID, localUser.Username, ldapUser.Email, ldapUser.FirstName, ldapUser.LastName, "")
					localUser.Email = ldapUser.Email
					localUser.FirstName = ldapUser.FirstName
					localUser.LastName = ldapUser.LastName
				}
			}
			user = localUser
			if err := s.Repo.SyncLDAPGroups(user.ID, ldapUser.Groups); err != nil {
				log.Printf("Warning: Failed to sync LDAP groups for user %s: %v", user.Username, err)
			}
			if refreshedUser, getErr := s.Repo.GetUserByID(user.ID); getErr == nil {
				user = refreshedUser
			}
		} else {
			log.Printf("LDAP authentication failed for user: %s: %v. Falling back to local database...", req.Username, authErr)
		}
	}

	if !ldapAuthenticated {
		// Fallback to local DB check
		user, err = s.Repo.GetUserByUsername(req.Username)
		if err != nil {
			s.SecLogger.LogEvent(r.Context(), securitylog.Event{
				EventType: securitylog.DomainAuth,
				Action:    "LOGIN_FAILED",
				Severity:  securitylog.SeverityWarn,
				Actor:     req.Username,
				ActorIP:   securitylog.GetClientIP(r),
				UserAgent: r.UserAgent(),
				Status:    securitylog.StatusFailure,
				Details:   map[string]interface{}{"reason": "user not found"},
			})
			s.respondWithError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			s.SecLogger.LogEvent(r.Context(), securitylog.Event{
				EventType: securitylog.DomainAuth,
				Action:    "LOGIN_FAILED",
				Severity:  securitylog.SeverityWarn,
				Actor:     req.Username,
				ActorIP:   securitylog.GetClientIP(r),
				UserAgent: r.UserAgent(),
				Status:    securitylog.StatusFailure,
				Details:   map[string]interface{}{"reason": "invalid password"},
			})
			s.respondWithError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
	}

	// Generate access token (5 mins)
	resolvedRoles, resolvedPermissions, err := s.Repo.GetResolvedUserRolesAndPermissions(user.ID, "system-tenant", "icarus-auth-ms")
	if err != nil {
		resolvedRoles = []string{}
		resolvedPermissions = []string{}
	}
	ownedModules, err := s.Repo.GetUserOwnedModules(user.ID)
	if err != nil {
		ownedModules = []string{}
	}
	accessToken, err := s.TokenMgr.GenerateUserTokenWithRoles(user.Username, resolvedRoles, resolvedPermissions, ownedModules)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to generate access token")
		return
	}

	// Generate refresh token (24 hours)
	rawRefreshToken, err := generateSecureToken(32)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if err := s.Repo.StoreRefreshToken(rawRefreshToken, user.ID, expiresAt); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to store refresh token")
		return
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainAuth,
		Action:         "LOGIN_SUCCESS",
		Severity:       securitylog.SeverityInfo,
		Actor:          user.Username,
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: fmt.Sprintf("user:%d", user.ID),
		Status:         securitylog.StatusSuccess,
		Details:        map[string]interface{}{"auth_method": authMethod},
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"access_token":  accessToken,
		"refresh_token": rawRefreshToken,
	})
}

// RefreshHandler rotates the refresh token and issues a new access token.
func (s *HandlerServer) RefreshHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		s.respondWithError(w, http.StatusBadRequest, "refresh token is required")
		return
	}

	storedToken, err := s.Repo.GetRefreshToken(req.RefreshToken)
	if err != nil {
		s.SecLogger.LogEvent(r.Context(), securitylog.Event{
			EventType: securitylog.DomainAuth,
			Action:    "REFRESH_TOKEN",
			Severity:  securitylog.SeverityWarn,
			ActorIP:   securitylog.GetClientIP(r),
			UserAgent: r.UserAgent(),
			Status:    securitylog.StatusFailure,
			Details:   map[string]interface{}{"reason": "invalid token"},
		})
		s.respondWithError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	if storedToken.ExpiresAt.Before(time.Now()) {
		_ = s.Repo.DeleteRefreshToken(req.RefreshToken)
		s.SecLogger.LogEvent(r.Context(), securitylog.Event{
			EventType: securitylog.DomainAuth,
			Action:    "REFRESH_TOKEN",
			Severity:  securitylog.SeverityWarn,
			ActorIP:   securitylog.GetClientIP(r),
			UserAgent: r.UserAgent(),
			Status:    securitylog.StatusFailure,
			Details:   map[string]interface{}{"reason": "token expired"},
		})
		s.respondWithError(w, http.StatusUnauthorized, "refresh token has expired")
		return
	}

	user, err := s.Repo.GetUserByID(storedToken.UserID)
	if err != nil {
		s.respondWithError(w, http.StatusUnauthorized, "user no longer exists")
		return
	}

	// Token rotation: delete old refresh token
	if err := s.Repo.DeleteRefreshToken(req.RefreshToken); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "token rotation failed")
		return
	}

	// Generate new access token
	resolvedRoles, resolvedPermissions, err := s.Repo.GetResolvedUserRolesAndPermissions(user.ID, "system-tenant", "icarus-auth-ms")
	if err != nil {
		resolvedRoles = []string{}
		resolvedPermissions = []string{}
	}
	ownedModules, err := s.Repo.GetUserOwnedModules(user.ID)
	if err != nil {
		ownedModules = []string{}
	}
	newAccessToken, err := s.TokenMgr.GenerateUserTokenWithRoles(user.Username, resolvedRoles, resolvedPermissions, ownedModules)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to generate access token")
		return
	}

	// Generate new refresh token
	newRawRefreshToken, err := generateSecureToken(32)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	newExpiresAt := time.Now().Add(24 * time.Hour)
	if err := s.Repo.StoreRefreshToken(newRawRefreshToken, user.ID, newExpiresAt); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to store refresh token")
		return
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainAuth,
		Action:         "REFRESH_TOKEN",
		Severity:       securitylog.SeverityInfo,
		Actor:          user.Username,
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: fmt.Sprintf("user:%d", user.ID),
		Status:         securitylog.StatusSuccess,
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"access_token":  newAccessToken,
		"refresh_token": newRawRefreshToken,
	})
}
