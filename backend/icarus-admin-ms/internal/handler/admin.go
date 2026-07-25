package handler

import (
	"context"
	"icarus-admin-ms/internal/securitylog"
	"net/http"
	"strings"
)

type contextKey string

const claimsContextKey contextKey = "claims"

// AdminRequired middleware ensures the request has a valid admin or module owner JWT.
func (s *HandlerServer) AdminRequired(next http.HandlerFunc) http.HandlerFunc {
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
		claims, err := s.Verifier.VerifyToken(tokenStr)
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

		isAdmin := false
		for _, role := range claims.Roles {
			if role == "admin" {
				isAdmin = true
				break
			}
		}

		isModuleOwner := len(claims.OwnedModules) > 0

		if !isAdmin && !isModuleOwner {
			s.SecLogger.LogEvent(r.Context(), securitylog.Event{
				EventType:      securitylog.DomainAccessControl,
				Action:         "ACCESS_DENIED",
				Severity:       securitylog.SeverityWarn,
				Actor:          claims.Subject,
				ActorIP:        securitylog.GetClientIP(r),
				UserAgent:      r.UserAgent(),
				TargetResource: r.URL.Path,
				Status:         securitylog.StatusFailure,
				Details:        map[string]interface{}{"reason": "admin or module owner role required"},
			})
			s.respondWithError(w, http.StatusForbidden, "forbidden: admin or module owner role required")
			return
		}

		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next(w, r.WithContext(ctx))
	}
}
