package handler

import (
	"context"
	"encoding/json"
	"icarus-workflow-ms/internal/crypto"
	"icarus-workflow-ms/internal/repository"
	"log"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const claimsKey contextKey = "claims"

// HandlerServer holds shared dependencies for all handlers.
type HandlerServer struct {
	Repo       *repository.SQLRepository
	Verifier   *crypto.TokenVerifier
	AdminMSURL string
	SSEBroker  *SSEBroker
}

func NewHandlerServer(repo *repository.SQLRepository, verifier *crypto.TokenVerifier, adminMSURL string) *HandlerServer {
	return &HandlerServer{
		Repo:       repo,
		Verifier:   verifier,
		AdminMSURL: adminMSURL,
		SSEBroker:  NewSSEBroker(),
	}
}

// RegisterRoutes maps all workflow-ms routes to the mux.
func (s *HandlerServer) RegisterRoutes(mux *http.ServeMux) {
	// Cart lifecycle (auth required)
	mux.HandleFunc("POST /api/v1/access/carts", s.AuthRequired(s.CreateCartHandler))
	mux.HandleFunc("GET /api/v1/access/carts", s.AuthRequired(s.ListCartsHandler))
	mux.HandleFunc("GET /api/v1/access/carts/{cart_id}", s.AuthRequired(s.GetCartHandler))
	mux.HandleFunc("POST /api/v1/access/carts/{cart_id}/items", s.AuthRequired(s.AddCartItemHandler))
	mux.HandleFunc("DELETE /api/v1/access/carts/{cart_id}/items/{item_id}", s.AuthRequired(s.RemoveCartItemHandler))
	mux.HandleFunc("POST /api/v1/access/carts/{cart_id}/submit", s.AuthRequired(s.SubmitCartHandler))

	// Approver inbox (auth required)
	mux.HandleFunc("GET /api/v1/workflow/inbox", s.AuthRequired(s.GetInboxHandler))
	mux.HandleFunc("POST /api/v1/workflow/steps/{step_id}/approve", s.AuthRequired(s.ApproveStepHandler))
	mux.HandleFunc("POST /api/v1/workflow/steps/{step_id}/reject", s.AuthRequired(s.RejectStepHandler))
	mux.HandleFunc("POST /api/v1/workflow/steps/{step_id}/delegate", s.AuthRequired(s.DelegateStepHandler))

	// SSE (auth required — long-lived)
	mux.HandleFunc("GET /api/v1/workflow/events", s.AuthRequired(s.SSEHandler))

	// Audit
	mux.HandleFunc("GET /api/v1/workflow/instances/{instance_id}/audit-trail", s.AuthRequired(s.AuditTrailHandler))

	// Admin workflow definition management
	mux.HandleFunc("GET /api/v1/workflow/definitions", s.AdminRequired(s.ListWorkflowDefinitionsHandler))
	mux.HandleFunc("GET /api/v1/workflow/definitions/history/{definition_key}", s.AdminRequired(s.GetDefinitionHistoryHandler))
	mux.HandleFunc("GET /api/v1/workflow/definitions/roles/{role_id}", s.AuthRequired(s.GetRoleWorkflowHandler))
	mux.HandleFunc("PUT /api/v1/workflow/definitions/roles/{role_id}", s.AdminRequired(s.UpsertRoleWorkflowHandler))
}

// ─── Middleware ───────────────────────────────────────────────────────────────

// AuthRequired validates JWT and injects claims into context.
// For SSE connections (EventSource cannot set custom headers),
// the token may also be passed as a ?token= query parameter.
func (s *HandlerServer) AuthRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := ""

		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				tokenStr = parts[1]
			}
		}

		// Fallback: ?token= query param for SSE / EventSource clients
		if tokenStr == "" {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			s.respondWithError(w, http.StatusUnauthorized, "missing authorization")
			return
		}

		claims, err := s.Verifier.VerifyToken(tokenStr)
		if err != nil {
			s.respondWithError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

// AdminRequired wraps AuthRequired and additionally enforces admin role.
func (s *HandlerServer) AdminRequired(next http.HandlerFunc) http.HandlerFunc {
	return s.AuthRequired(func(w http.ResponseWriter, r *http.Request) {
		claims := r.Context().Value(claimsKey).(*crypto.CustomClaims)
		isAdmin := false
		for _, role := range claims.Roles {
			if role == "admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin && len(claims.OwnedModules) == 0 {
			s.respondWithError(w, http.StatusForbidden, "admin or module owner role required")
			return
		}
		next(w, r)
	})
}

func (s *HandlerServer) claimsFrom(r *http.Request) *crypto.CustomClaims {
	claims, _ := r.Context().Value(claimsKey).(*crypto.CustomClaims)
	return claims
}

// ─── Middleware: CORS & Logging ───────────────────────────────────────────────

func (s *HandlerServer) LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[Workflow MS] %s %s %s - %s", r.Method, r.URL.Path, r.Proto, time.Since(start))
	})
}

func (s *HandlerServer) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Cache-Control")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (s *HandlerServer) respondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(response)
}

func (s *HandlerServer) respondWithError(w http.ResponseWriter, status int, message string) {
	s.respondWithJSON(w, status, map[string]string{"error": message})
}

func (s *HandlerServer) decodeJSON(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}
