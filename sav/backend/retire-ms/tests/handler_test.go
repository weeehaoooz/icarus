package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"retire-ms/internal/calculator"
	"retire-ms/internal/handler"
	"testing"
)

func TestCalculateHandler_Success(t *testing.T) {
	server := handler.NewHandlerServer()
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	payload := `{
		"current_age": 30,
		"retirement_age": 60,
		"life_expectancy": 85,
		"current_monthly_expenses": 5000.0,
		"current_savings": 50000.0,
		"expected_inflation_rate": 3.0,
		"expected_return_before_retirement": 7.0,
		"expected_return_during_retirement": 4.0
	}`

	req := httptest.NewRequest("POST", "/calculate", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var report calculator.CalculationOutput
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("Failed to decode response JSON: %v", err)
	}

	// Verify details
	if report.YearsToRetirement != 30 {
		t.Errorf("Expected 30 years to retirement, got %d", report.YearsToRetirement)
	}
	if report.RetirementDurationYears != 25 {
		t.Errorf("Expected 25 retirement years, got %d", report.RetirementDurationYears)
	}
	if len(report.Notes) == 0 {
		t.Errorf("Expected notes to be populated")
	}
}

func TestCalculateHandler_InvalidMethod(t *testing.T) {
	server := handler.NewHandlerServer()
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/calculate", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code 405, got %d", rr.Code)
	}
}

func TestCalculateHandler_ValidationErrors(t *testing.T) {
	tests := []struct {
		name         string
		payload      string
		expectedCode int
	}{
		{
			"zero current_age",
			`{"current_age": 0, "retirement_age": 60, "life_expectancy": 85}`,
			http.StatusBadRequest,
		},
		{
			"retirement_age less than current_age",
			`{"current_age": 40, "retirement_age": 30, "life_expectancy": 85}`,
			http.StatusBadRequest,
		},
		{
			"life_expectancy less than retirement_age",
			`{"current_age": 30, "retirement_age": 60, "life_expectancy": 50}`,
			http.StatusBadRequest,
		},
		{
			"negative monthly expenses",
			`{"current_age": 30, "retirement_age": 60, "life_expectancy": 85, "current_monthly_expenses": -100}`,
			http.StatusBadRequest,
		},
		{
			"negative savings",
			`{"current_age": 30, "retirement_age": 60, "life_expectancy": 85, "current_savings": -1000}`,
			http.StatusBadRequest,
		},
		{
			"invalid json structure",
			`{"current_age": "thirty"}`,
			http.StatusBadRequest,
		},
	}

	server := handler.NewHandlerServer()
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/calculate", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != tt.expectedCode {
				t.Errorf("Expected status code %d, got %d. Body: %s", tt.expectedCode, rr.Code, rr.Body.String())
			}
		})
	}
}
