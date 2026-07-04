package handler

import (
	"encoding/json"
	"sav-platform-ms/internal/crypto"
	"sav-platform-ms/internal/repository"
	"log"
	"net/http"
	"time"
)

// HandlerServer holds all shared dependencies for finance-ms handlers.
type HandlerServer struct {
	Repo     *repository.SQLRepository
	Verifier *crypto.TokenVerifier
}

func NewHandlerServer(repo *repository.SQLRepository, verifier *crypto.TokenVerifier) *HandlerServer {
	return &HandlerServer{Repo: repo, Verifier: verifier}
}

// RegisterRoutes maps all finance-ms routes to the multiplexer.
func (s *HandlerServer) RegisterRoutes(mux *http.ServeMux) {
	// Summary
	mux.HandleFunc("GET /api/v1/finance/summary", s.AuthRequired(s.FinanceGetSummaryHandler))

	// Assets
	mux.HandleFunc("GET /api/v1/finance/assets",          s.AuthRequired(s.FinanceListAssetsHandler))
	mux.HandleFunc("POST /api/v1/finance/assets",         s.AuthRequired(s.FinanceCreateAssetHandler))
	mux.HandleFunc("PATCH /api/v1/finance/assets/{id}",   s.AuthRequired(s.FinancePatchAssetHandler))
	mux.HandleFunc("DELETE /api/v1/finance/assets/{id}",  s.AuthRequired(s.FinanceDeleteAssetHandler))
	mux.HandleFunc("POST /api/v1/finance/assets/import",  s.AuthRequired(s.FinanceImportAssetsHandler))

	// Live equity prices
	mux.HandleFunc("GET /api/v1/finance/prices", s.AuthRequired(s.FinanceGetPricesHandler))

	// Liabilities
	mux.HandleFunc("GET /api/v1/finance/liabilities",         s.AuthRequired(s.FinanceListLiabilitiesHandler))
	mux.HandleFunc("POST /api/v1/finance/liabilities",        s.AuthRequired(s.FinanceCreateLiabilityHandler))
	mux.HandleFunc("PATCH /api/v1/finance/liabilities/{id}",  s.AuthRequired(s.FinancePatchLiabilityHandler))
	mux.HandleFunc("DELETE /api/v1/finance/liabilities/{id}", s.AuthRequired(s.FinanceDeleteLiabilityHandler))

	// Insights
	mux.HandleFunc("GET /api/v1/finance/insights",                   s.AuthRequired(s.FinanceListInsightsHandler))
	mux.HandleFunc("PATCH /api/v1/finance/insights/{id}/read",       s.AuthRequired(s.FinanceMarkInsightReadHandler))
	mux.HandleFunc("PATCH /api/v1/finance/insights/{id}/dismiss",    s.AuthRequired(s.FinanceDismissInsightHandler))
	mux.HandleFunc("POST /api/v1/finance/insights/recompute",        s.AuthRequired(s.FinanceRecomputeInsightsHandler))

	// Preferences
	mux.HandleFunc("GET /api/v1/finance/preferences", s.AuthRequired(s.FinanceGetPreferencesHandler))
	mux.HandleFunc("PUT /api/v1/finance/preferences", s.AuthRequired(s.FinanceUpdatePreferencesHandler))
}

// LoggerMiddleware logs request method, path, and latency.
func (s *HandlerServer) LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[Finance] %s %s %s - %s", r.Method, r.URL.Path, r.Proto, time.Since(start))
	})
}

// CORSMiddleware allows cross-origin requests from the frontend.
func (s *HandlerServer) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// respondWithJSON writes a JSON payload with the given status code.
func (s *HandlerServer) respondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(response)
}

// respondWithError writes a JSON error payload.
func (s *HandlerServer) respondWithError(w http.ResponseWriter, status int, message string) {
	s.respondWithJSON(w, status, map[string]string{"error": message})
}
