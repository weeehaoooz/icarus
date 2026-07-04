package handler

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"retire-ms/internal/calculator"
	"time"
)

type HandlerServer struct{}

func NewHandlerServer() *HandlerServer {
	return &HandlerServer{}
}

// RegisterRoutes maps all Retire Server routes to the multiplexer.
func (s *HandlerServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /calculate", s.CalculateHandler)
}

// LoggerMiddleware logs requests and their latency.
func (s *HandlerServer) LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s - %s", r.Method, r.URL.Path, r.Proto, time.Since(start))
	})
}

// CalculateHandler parses input parameters, validates them, normalizes interest rates, and returns the retirement projection.
func (s *HandlerServer) CalculateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed. Only POST requests are accepted on this endpoint.")
		return
	}

	var input calculator.CalculationInput
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&input); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "Invalid JSON input structure: "+err.Error())
		return
	}

	// Validation
	if input.CurrentAge <= 0 {
		s.respondWithError(w, http.StatusBadRequest, "current_age must be a positive integer greater than 0")
		return
	}
	if input.RetirementAge < input.CurrentAge {
		s.respondWithError(w, http.StatusBadRequest, "retirement_age must be greater than or equal to current_age")
		return
	}
	if input.LifeExpectancy < input.RetirementAge {
		s.respondWithError(w, http.StatusBadRequest, "life_expectancy must be greater than or equal to retirement_age")
		return
	}
	if input.CurrentMonthlyExpenses < 0 {
		s.respondWithError(w, http.StatusBadRequest, "current_monthly_expenses cannot be negative")
		return
	}
	if input.CurrentSavings < 0 {
		s.respondWithError(w, http.StatusBadRequest, "current_savings cannot be negative")
		return
	}

	// Normalize rates (if user inputs e.g. 5.0 instead of 0.05, convert it)
	input.ExpectedInflationRate = normalizeRate(input.ExpectedInflationRate)
	input.ExpectedReturnBeforeRetirement = normalizeRate(input.ExpectedReturnBeforeRetirement)
	input.ExpectedReturnDuringRetirement = normalizeRate(input.ExpectedReturnDuringRetirement)

	// Generate the report
	report := calculator.GenerateReport(input)

	s.respondWithJSON(w, http.StatusOK, report)
}

// normalizeRate converts percentage values (e.g. 5.5 for 5.5%) into decimal format (e.g. 0.055).
// It assumes any rate with absolute value greater than 1.0 is intended to be a percentage.
func normalizeRate(rate float64) float64 {
	if math.Abs(rate) > 1.0 {
		return rate / 100.0
	}
	return rate
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
