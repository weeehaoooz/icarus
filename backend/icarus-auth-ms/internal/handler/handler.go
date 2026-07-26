package handler

import (
	"encoding/json"
	"icarus-auth-ms/internal/crypto"
	"icarus-auth-ms/internal/repository"
	"icarus-auth-ms/internal/securitylog"
	"log"
	"net/http"
	"time"
)

type HandlerServer struct {
	Repo      *repository.SQLRepository
	TokenMgr  *crypto.TokenManager
	SecLogger *securitylog.Logger
}

func NewHandlerServer(repo *repository.SQLRepository, tokenMgr *crypto.TokenManager) *HandlerServer {
	return &HandlerServer{
		Repo:      repo,
		TokenMgr:  tokenMgr,
		SecLogger: securitylog.NewLogger("icarus-auth-ms"),
	}
}

// RegisterRoutes maps all Auth Server routes to the multiplexer.
func (s *HandlerServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /register", s.RegisterHandler)
	mux.HandleFunc("POST /login", s.LoginHandler)
	mux.HandleFunc("POST /refresh", s.RefreshHandler)
	mux.HandleFunc("POST /logout", s.LogoutHandler)

	mux.HandleFunc("POST /client/register", s.ClientRegisterHandler)
	mux.HandleFunc("POST /client/token", s.ClientTokenHandler)
	mux.HandleFunc("GET /certs", s.CertsHandler)
	mux.HandleFunc("GET /verify", s.VerifyTokenHandler)
	mux.HandleFunc("POST /internal/modules/sync", s.InternalSyncModuleHandler)
	mux.HandleFunc("DELETE /internal/modules/sync/{id}", s.InternalDeleteModuleHandler)
	mux.HandleFunc("GET /internal/modules/{id}/roles-and-templates", s.InternalGetModuleRolesAndTemplatesHandler)
	mux.HandleFunc("POST /internal/tenants/sync", s.InternalSyncTenantHandler)
	mux.HandleFunc("DELETE /internal/tenants/sync/{id}", s.InternalDeleteTenantHandler)
	mux.HandleFunc("POST /internal/applications/sync", s.InternalSyncApplicationHandler)
	mux.HandleFunc("DELETE /internal/applications/sync/{id}", s.InternalDeleteApplicationHandler)
	mux.HandleFunc("POST /internal/modules/{id}/applications/sync", s.InternalSyncOnboardApplicationHandler)
	mux.HandleFunc("DELETE /internal/modules/{id}/applications/sync/{appCode}", s.InternalSyncOffboardApplicationHandler)
	mux.HandleFunc("GET /internal/roles/{role_name}/members", s.InternalListRoleMembersHandler)
	mux.HandleFunc("POST /token/scope", s.TokenScopeHandler)

	// Profile and password APIs for authenticated users
	mux.HandleFunc("GET /me", s.UserRequired(s.MeGetHandler))
	mux.HandleFunc("GET /me/roles", s.UserRequired(s.MeRolesHandler))
	mux.HandleFunc("GET /me/permissions", s.UserRequired(s.MePermissionsHandler))
	mux.HandleFunc("GET /roles", s.UserRequired(s.AdminListRolesHandler))
	mux.HandleFunc("PUT /me", s.UserRequired(s.MeUpdateHandler))
	mux.HandleFunc("PUT /me/password", s.UserRequired(s.MeChangePasswordHandler))

	// User directory endpoints — accessible by all authenticated users, exposes only safe fields
	mux.HandleFunc("GET /users", s.UserRequired(s.UserDirectoryHandler))
	mux.HandleFunc("GET /users/{id}/roles", s.UserRequired(s.UserRolesPublicHandler))

	// Admin APIs for RBAC CRM
	mux.HandleFunc("GET /admin/users", s.AdminRequired(s.AdminListUsersHandler))
	mux.HandleFunc("POST /admin/users", s.AdminRequired(s.AdminCreateUserHandler))
	mux.HandleFunc("GET /admin/users/{id}", s.AdminRequired(s.AdminGetUserHandler))
	mux.HandleFunc("PUT /admin/users/{id}", s.AdminRequired(s.AdminUpdateUserHandler))
	mux.HandleFunc("DELETE /admin/users/{id}", s.AdminRequired(s.AdminDeleteUserHandler))

	mux.HandleFunc("GET /admin/clients", s.AdminRequired(s.AdminListClientsHandler))
	mux.HandleFunc("POST /admin/clients", s.AdminRequired(s.AdminCreateClientHandler))
	mux.HandleFunc("GET /admin/clients/{id}", s.AdminRequired(s.AdminGetClientHandler))
	mux.HandleFunc("PUT /admin/clients/{id}", s.AdminRequired(s.AdminUpdateClientHandler))
	mux.HandleFunc("DELETE /admin/clients/{id}", s.AdminRequired(s.AdminDeleteClientHandler))

	mux.HandleFunc("GET /admin/roles", s.AdminRequired(s.AdminListRolesHandler))
	mux.HandleFunc("POST /admin/roles", s.AdminRequired(s.AdminCreateRoleHandler))
	mux.HandleFunc("PUT /admin/roles/{id}", s.AdminRequired(s.AdminUpdateRoleHandler))
	mux.HandleFunc("DELETE /admin/roles/{id}", s.AdminRequired(s.AdminDeleteRoleHandler))

	mux.HandleFunc("GET /admin/applications", s.AdminRequired(s.AdminListApplicationsHandler))
	mux.HandleFunc("GET /admin/permissions", s.AdminRequired(s.AdminListPermissionsHandler))

	// LDAP configuration endpoints
	mux.HandleFunc("GET /admin/ldap-config", s.AdminRequired(s.AdminGetLDAPConfigHandler))
	mux.HandleFunc("PUT /admin/ldap-config", s.AdminRequired(s.AdminUpdateLDAPConfigHandler))
	mux.HandleFunc("POST /admin/ldap-config/test", s.AdminRequired(s.AdminTestLDAPConfigHandler))
}

// LoggerMiddleware logs requests and their latency.
func (s *HandlerServer) LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[Auth MS] %s %s %s - %s", r.Method, r.URL.Path, r.Proto, time.Since(start))
	})
}

// Helper to send JSON responses
func (s *HandlerServer) respondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(response)
}

// Helper to send JSON errors
func (s *HandlerServer) respondWithError(w http.ResponseWriter, status int, message string) {
	s.respondWithJSON(w, status, map[string]string{"error": message})
}

// CORSMiddleware handles cross-origin requests from the frontend CRM.
func (s *HandlerServer) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
