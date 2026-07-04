package tests

import (
	"math"
	"retire-ms/internal/calculator"
	"testing"
)

func TestGenerateReport(t *testing.T) {
	input := calculator.CalculationInput{
		CurrentAge:                     30,
		RetirementAge:                  35,
		LifeExpectancy:                 37,
		CurrentMonthlyExpenses:         1000.0,
		CurrentSavings:                 10000.0,
		ExpectedInflationRate:          0.05,
		ExpectedReturnBeforeRetirement: 0.08,
		ExpectedReturnDuringRetirement: 0.06,
	}

	report := calculator.GenerateReport(input)

	// Validate basic durations
	if report.YearsToRetirement != 5 {
		t.Errorf("Expected 5 years to retirement, got %d", report.YearsToRetirement)
	}
	if report.RetirementDurationYears != 2 {
		t.Errorf("Expected 2 retirement duration years, got %d", report.RetirementDurationYears)
	}

	// Validate annual expenses
	expectedAnnualExpenseNow := 12000.0
	if math.Abs(report.AnnualExpenseNow-expectedAnnualExpenseNow) > 1e-4 {
		t.Errorf("Expected AnnualExpenseNow to be %f, got %f", expectedAnnualExpenseNow, report.AnnualExpenseNow)
	}

	expectedAnnualExpenseAtRetirement := 12000.0 * math.Pow(1.05, 5)
	if math.Abs(report.AnnualExpenseAtRetirement-expectedAnnualExpenseAtRetirement) > 1e-4 {
		t.Errorf("Expected AnnualExpenseAtRetirement to be %f, got %f", expectedAnnualExpenseAtRetirement, report.AnnualExpenseAtRetirement)
	}

	// Validate Target Nest Egg
	realRate := 1.06/1.05 - 1
	expectedNestEgg := expectedAnnualExpenseAtRetirement * (1 - math.Pow(1+realRate, -2)) / (1 - math.Pow(1+realRate, -1))
	if math.Abs(report.TargetNestEgg-expectedNestEgg) > 1e-2 {
		t.Errorf("Expected TargetNestEgg to be %f, got %f", expectedNestEgg, report.TargetNestEgg)
	}

	// Validate Projections lengths
	if len(report.Projections) != 5 {
		t.Errorf("Expected 5 years in accumulation projections, got %d", len(report.Projections))
	}
	if len(report.RetirementProjections) != 2 {
		t.Errorf("Expected 2 years in retirement projections, got %d", len(report.RetirementProjections))
	}

	// Validate ending balances
	if len(report.Projections) > 0 {
		finalAccumulationBalance := report.Projections[len(report.Projections)-1].EndingBalance
		if math.Abs(finalAccumulationBalance-report.TargetNestEgg) > 1e-2 {
			t.Errorf("Expected final accumulation balance (%f) to match target nest egg (%f)", finalAccumulationBalance, report.TargetNestEgg)
		}
	}

	if len(report.RetirementProjections) > 0 {
		finalRetirementBalance := report.RetirementProjections[len(report.RetirementProjections)-1].EndingBalance
		if math.Abs(finalRetirementBalance-0) > 1e-2 {
			t.Errorf("Expected final retirement balance to be 0, got %f", finalRetirementBalance)
		}
	}

	// Validate notes presence
	if len(report.Notes) == 0 {
		t.Errorf("Expected notes to be populated, got empty list")
	}
}

func TestGenerateReport_NoShortfall(t *testing.T) {
	// Savings are extremely high, so there is no shortfall
	input := calculator.CalculationInput{
		CurrentAge:                     30,
		RetirementAge:                  35,
		LifeExpectancy:                 37,
		CurrentMonthlyExpenses:         10.0, // $120/year
		CurrentSavings:                 10000.0,
		ExpectedInflationRate:          0.03,
		ExpectedReturnBeforeRetirement: 0.05,
		ExpectedReturnDuringRetirement: 0.04,
	}

	report := calculator.GenerateReport(input)

	if report.NestEggShortfall != 0 {
		t.Errorf("Expected NestEggShortfall to be 0, got %f", report.NestEggShortfall)
	}
	if report.RequiredAnnualSavings != 0 || report.RequiredMonthlySavings != 0 {
		t.Errorf("Expected savings required to be 0, got annual=%f, monthly=%f", report.RequiredAnnualSavings, report.RequiredMonthlySavings)
	}
}
