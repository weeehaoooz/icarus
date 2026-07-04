package tests

import (
	"math"
	"retire-ms/internal/calculator"
	"testing"
)

func TestAdjustForInflation(t *testing.T) {
	tests := []struct {
		name     string
		amount   float64
		rate     float64
		years    int
		expected float64
	}{
		{"zero years", 1000.0, 0.03, 0, 1000.0},
		{"negative years", 1000.0, 0.03, -5, 1000.0},
		{"zero inflation rate", 1000.0, 0.0, 10, 1000.0},
		{"standard scenario", 1000.0, 0.03, 2, 1060.9}, // 1000 * (1.03)^2 = 1060.9
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculator.AdjustForInflation(tt.amount, tt.rate, tt.years)
			if math.Abs(got-tt.expected) > 1e-4 {
				t.Errorf("AdjustForInflation(%v, %v, %v) = %v; expected %v", tt.amount, tt.rate, tt.years, got, tt.expected)
			}
		})
	}
}

func TestCalculateFutureValue(t *testing.T) {
	tests := []struct {
		name      string
		principal float64
		rate      float64
		years     int
		expected  float64
	}{
		{"zero years", 10000.0, 0.07, 0, 10000.0},
		{"negative years", 10000.0, 0.07, -1, 10000.0},
		{"standard scenario", 1000.0, 0.08, 5, 1469.3280768}, // 1000 * (1.08)^5
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculator.CalculateFutureValue(tt.principal, tt.rate, tt.years)
			if math.Abs(got-tt.expected) > 1e-4 {
				t.Errorf("CalculateFutureValue(%v, %v, %v) = %v; expected %v", tt.principal, tt.rate, tt.years, got, tt.expected)
			}
		})
	}
}

func TestCalculateAnnuityDuePV(t *testing.T) {
	tests := []struct {
		name          string
		annualExpense float64
		inflationRate float64
		returnRate    float64
		years         int
		expected      float64
	}{
		{"zero years", 50000.0, 0.03, 0.06, 0, 0.0},
		{"negative years", 50000.0, 0.03, 0.06, -5, 0.0},
		{"real rate zero", 10000.0, 0.03, 0.03, 5, 50000.0}, // 10000 * 5 when real return is zero
		{"standard scenario 2 years", 10000.0, 0.03, 0.06, 2, 19716.981132}, // 10000 * (1 + 1.03/1.06)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculator.CalculateAnnuityDuePV(tt.annualExpense, tt.inflationRate, tt.returnRate, tt.years)
			if math.Abs(got-tt.expected) > 1e-4 {
				t.Errorf("CalculateAnnuityDuePV(%v, %v, %v, %v) = %v; expected %v", tt.annualExpense, tt.inflationRate, tt.returnRate, tt.years, got, tt.expected)
			}
		})
	}
}

func TestCalculateRequiredSavings(t *testing.T) {
	tests := []struct {
		name           string
		shortfall      float64
		rate           float64
		years          int
		expectedAnnual float64
		expectedMonth  float64
	}{
		{"zero years", 100000.0, 0.07, 0, 0.0, 0.0},
		{"zero shortfall", 0.0, 0.07, 10, 0.0, 0.0},
		{"zero rate", 100000.0, 0.0, 10, 10000.0, 833.333333},
		{"standard scenario", 100000.0, 0.05, 10, 7950.457496, 647.822967},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAnnual, gotMonthly := calculator.CalculateRequiredSavings(tt.shortfall, tt.rate, tt.years)
			if math.Abs(gotAnnual-tt.expectedAnnual) > 1e-4 {
				t.Errorf("CalculateRequiredSavings annual shortfall=%v, rate=%v, years=%v got %v; expected %v", tt.shortfall, tt.rate, tt.years, gotAnnual, tt.expectedAnnual)
			}
			if math.Abs(gotMonthly-tt.expectedMonth) > 1e-4 {
				t.Errorf("CalculateRequiredSavings monthly shortfall=%v, rate=%v, years=%v got %v; expected %v", tt.shortfall, tt.rate, tt.years, gotMonthly, tt.expectedMonth)
			}
		})
	}
}

func TestCalculateEffectiveMonthlyRate(t *testing.T) {
	tests := []struct {
		name       string
		annualRate float64
		expected   float64
	}{
		{"zero rate", 0.0, 0.0},
		{"standard rate 6%", 0.06, 0.004867550565},
		{"invalid rate", -1.5, -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculator.CalculateEffectiveMonthlyRate(tt.annualRate)
			if math.Abs(got-tt.expected) > 1e-9 {
				t.Errorf("CalculateEffectiveMonthlyRate(%v) = %v; expected %v", tt.annualRate, got, tt.expected)
			}
		})
	}
}
