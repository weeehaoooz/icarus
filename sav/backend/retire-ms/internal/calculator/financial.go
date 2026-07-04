package calculator

import (
	"math"
)

// AdjustForInflation calculates the future value of a current cost/amount
// based on a constant inflation rate over a given number of years.
// Formula: FV = PV * (1 + inflationRate)^years
func AdjustForInflation(amount float64, rate float64, years int) float64 {
	if years <= 0 {
		return amount
	}
	return amount * math.Pow(1+rate, float64(years))
}

// CalculateFutureValue computes the compound growth of a one-time principal deposit.
// Formula: FV = PV * (1 + rate)^years
func CalculateFutureValue(principal float64, rate float64, years int) float64 {
	if years <= 0 {
		return principal
	}
	return principal * math.Pow(1+rate, float64(years))
}

// CalculateAnnuityDuePV computes the present value of a growing annuity due (payments at
// the beginning of each year) where the payment grows with inflation and the underlying
// balance grows with the expected investment return during retirement.
//
// Let r_post be the return rate during retirement, i be the inflation rate.
// The real rate of return during retirement is: r_real = (1 + r_post) / (1 + i) - 1
//
// Since it's an annuity due (withdrawals at the start of each year):
// PV_due = annualExpense * (1 - (1 + r_real)^(-years)) / (1 - (1 + r_real)^(-1))
func CalculateAnnuityDuePV(annualExpense float64, inflationRate float64, returnRate float64, years int) float64 {
	if years <= 0 {
		return 0
	}

	// Calculate the real rate of return during retirement
	realRate := (1+returnRate)/(1+inflationRate) - 1

	// Avoid division by zero when real rate is 0 (or close to 0 due to float precision)
	if math.Abs(realRate) < 1e-9 {
		return annualExpense * float64(years)
	}

	// Standard growing annuity due formula:
	// PV = PMT * (1 - (1 + r)^(-n)) / (1 - (1 + r)^(-1))
	numerator := 1 - math.Pow(1+realRate, -float64(years))
	denominator := 1 - math.Pow(1+realRate, -1)
	return annualExpense * (numerator / denominator)
}

// CalculateRequiredSavings calculates the annual and monthly savings required to accumulate
// a target shortfall amount over a given number of years at a specific rate of return.
// It assumes contributions are made at the end of each period (ordinary annuity).
func CalculateRequiredSavings(shortfall float64, annualRate float64, years int) (annual float64, monthly float64) {
	if shortfall <= 0 || years <= 0 {
		return 0, 0
	}

	// 1. Annual savings required (ordinary annuity compounding annually)
	if math.Abs(annualRate) < 1e-9 {
		annual = shortfall / float64(years)
	} else {
		// S_annual = Shortfall * r / ((1 + r)^n - 1)
		annual = shortfall * annualRate / (math.Pow(1+annualRate, float64(years)) - 1)
	}

	// 2. Monthly savings required (ordinary annuity compounding monthly using effective monthly rate)
	monthlyRate := CalculateEffectiveMonthlyRate(annualRate)
	months := years * 12
	if math.Abs(monthlyRate) < 1e-9 {
		monthly = shortfall / float64(months)
	} else {
		// S_monthly = Shortfall * r_monthly / ((1 + r_monthly)^(n*12) - 1)
		monthly = shortfall * monthlyRate / (math.Pow(1+monthlyRate, float64(months)) - 1)
	}

	return annual, monthly
}

// CalculateEffectiveMonthlyRate converts an annual compounding rate to its equivalent effective monthly rate.
// Formula: (1 + annualRate)^(1/12) - 1
func CalculateEffectiveMonthlyRate(annualRate float64) float64 {
	if annualRate <= -1.0 {
		return -1.0
	}
	return math.Pow(1+annualRate, 1.0/12.0) - 1
}
