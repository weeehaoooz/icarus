package handler

import (
	"encoding/json"
	"icarus-admin-ms/internal/crypto"
	"icarus-admin-ms/internal/repository"
	"log"
	"net/http"
	"time"
)

type HandlerServer struct {
	Repo      *repository.SQLRepository
	Verifier  *crypto.TokenVerifier
	AuthMSURL string
}

func NewHandlerServer(repo *repository.SQLRepository, verifier *crypto.TokenVerifier, authMSURL string) *HandlerServer {
	return &HandlerServer{
		Repo:      repo,
		Verifier:  verifier,
		AuthMSURL: authMSURL,
	}
}

// RegisterRoutes maps all platform-ms routes to the multiplexer.
func (s *HandlerServer) RegisterRoutes(mux *http.ServeMux) {
	// Gateway route (handles reverse proxy and rate limits)
	mux.HandleFunc("/gateway/{site_domain}/{path...}", s.GatewayHandler)

	// Dynamic onboarding route
	mux.HandleFunc("POST /api/v1/governance/modules/register", s.OnboardModuleHandler)

	// Admin APIs for Tenants
	mux.HandleFunc("GET /admin/tenants", s.AdminRequired(s.AdminListTenantsHandler))
	mux.HandleFunc("POST /admin/tenants", s.AdminRequired(s.AdminCreateTenantHandler))
	mux.HandleFunc("PUT /admin/tenants/{id}", s.AdminRequired(s.AdminUpdateTenantHandler))
	mux.HandleFunc("DELETE /admin/tenants/{id}", s.AdminRequired(s.AdminDeleteTenantHandler))

	// Admin APIs for Modules
	mux.HandleFunc("GET /admin/modules", s.AdminRequired(s.AdminListModulesHandler))
	mux.HandleFunc("POST /admin/modules", s.AdminRequired(s.AdminCreateModuleHandler))
	mux.HandleFunc("PUT /admin/modules/{id}", s.AdminRequired(s.AdminUpdateModuleHandler))
	mux.HandleFunc("DELETE /admin/modules/{id}", s.AdminRequired(s.AdminDeleteModuleHandler))
	mux.HandleFunc("GET /admin/modules/{id}/manifest", s.AdminRequired(s.AdminGetModuleManifestHandler))

	// Admin APIs for Applications
	mux.HandleFunc("GET /admin/applications", s.AdminRequired(s.AdminListApplicationsHandler))
	mux.HandleFunc("POST /admin/applications", s.AdminRequired(s.AdminCreateApplicationHandler))
	mux.HandleFunc("PUT /admin/applications/{id}", s.AdminRequired(s.AdminUpdateApplicationHandler))
	mux.HandleFunc("DELETE /admin/applications/{id}", s.AdminRequired(s.AdminDeleteApplicationHandler))

	// Admin APIs for Module Application Onboarding
	mux.HandleFunc("GET /admin/modules/{id}/applications", s.AdminRequired(s.AdminListModuleApplicationsHandler))
	mux.HandleFunc("POST /admin/modules/{id}/applications", s.AdminRequired(s.AdminOnboardApplicationHandler))
	mux.HandleFunc("DELETE /admin/modules/{id}/applications/{appCode}", s.AdminRequired(s.AdminOffboardApplicationHandler))
}

// LoggerMiddleware logs requests and latency.
func (s *HandlerServer) LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[Platform Gateway] %s %s %s - %s", r.Method, r.URL.Path, r.Proto, time.Since(start))
	})
}

// CORSMiddleware handles CORS headers.
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

// Helper to send JSON response.
func (s *HandlerServer) respondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(response)
}

// Helper to send JSON error.
func (s *HandlerServer) respondWithError(w http.ResponseWriter, status int, message string) {
	s.respondWithJSON(w, status, map[string]string{"error": message})
}
