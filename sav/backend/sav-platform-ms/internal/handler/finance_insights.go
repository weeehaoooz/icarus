package handler

import (
	"sav-platform-ms/internal/models"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var insightThresholds = struct {
	IdleCashPct      decimal.Decimal
	ConcentrationPct decimal.Decimal
	DebtToEquity     decimal.Decimal
	ConservativePct  decimal.Decimal
}{
	IdleCashPct:      decimal.NewFromFloat(0.20),
	ConcentrationPct: decimal.NewFromFloat(0.40),
	DebtToEquity:     decimal.NewFromFloat(0.50),
	ConservativePct:  decimal.NewFromFloat(0.70),
}

// ComputeInsights derives actionable insights from a user's current portfolio.
// Pure function — no DB calls. Caller is responsible for persistence.
func ComputeInsights(userID string, assets []models.Asset, liabilities []models.Liability) []models.Insight {
	if len(assets) == 0 && len(liabilities) == 0 {
		return nil
	}

	totalAssets := decimal.Zero
	totalCash := decimal.Zero
	totalBonds := decimal.Zero
	totalLiabilities := decimal.Zero
	assetByClass := map[models.AssetClass]decimal.Decimal{}

	for _, a := range assets {
		totalAssets = totalAssets.Add(a.CurrentValue)
		assetByClass[a.AssetClass] = assetByClass[a.AssetClass].Add(a.CurrentValue)
		if a.AssetClass == models.AssetClassCash {
			totalCash = totalCash.Add(a.CurrentValue)
		}
		if a.AssetClass == models.AssetClassBond {
			totalBonds = totalBonds.Add(a.CurrentValue)
		}
	}
	for _, l := range liabilities {
		totalLiabilities = totalLiabilities.Add(l.OutstandingAmt)
	}

	var insights []models.Insight

	if totalAssets.IsPositive() {
		// Rule 1: Idle Cash
		cashPct := totalCash.Div(totalAssets)
		if cashPct.GreaterThan(insightThresholds.IdleCashPct) {
			pct := cashPct.Mul(decimal.NewFromInt(100)).StringFixed(1)
			insights = append(insights, models.Insight{
				ID:          uuid.New().String(),
				UserID:      userID,
				Category:    models.InsightCategoryIdleCash,
				Severity:    models.InsightSeverityWarning,
				Title:       "High idle cash detected",
				Body:        fmt.Sprintf("%s%% of your portfolio is held in cash. Consider deploying capital into equities or bonds.", pct),
				ActionURL:   "/assets",
				MetadataRaw: fmt.Sprintf(`{"cash_pct":"%s","threshold_pct":"20"}`, pct),
			})
		}

		// Rule 2: Concentration Risk
		for _, a := range assets {
			if a.CurrentValue.IsZero() {
				continue
			}
			pct := a.CurrentValue.Div(totalAssets)
			if pct.GreaterThan(insightThresholds.ConcentrationPct) {
				pctStr := pct.Mul(decimal.NewFromInt(100)).StringFixed(1)
				insights = append(insights, models.Insight{
					ID:          uuid.New().String(),
					UserID:      userID,
					Category:    models.InsightCategoryConcentration,
					Severity:    models.InsightSeverityWarning,
					Title:       fmt.Sprintf("Concentration risk: %s", a.Name),
					Body:        fmt.Sprintf("%s represents %s%% of your portfolio. Consider diversifying.", a.Name, pctStr),
					ActionURL:   "/assets",
					MetadataRaw: fmt.Sprintf(`{"asset_id":"%s","asset_name":"%s","pct":"%s"}`, a.ID, a.Name, pctStr),
				})
			}
		}

		// Rule 3: High Leverage
		dte := totalLiabilities.Div(totalAssets)
		if dte.GreaterThan(insightThresholds.DebtToEquity) {
			insights = append(insights, models.Insight{
				ID:          uuid.New().String(),
				UserID:      userID,
				Category:    models.InsightCategoryLeverage,
				Severity:    models.InsightSeverityCritical,
				Title:       "High leverage ratio",
				Body:        fmt.Sprintf("Debt-to-equity ratio is %sx (above 0.5x threshold). Prioritise repaying high-interest liabilities.", dte.StringFixed(2)),
				ActionURL:   "/liabilities",
				MetadataRaw: fmt.Sprintf(`{"debt_to_equity":"%s"}`, dte.StringFixed(2)),
			})
		}

		// Rule 4: Overly Conservative
		conservativePct := totalCash.Add(totalBonds).Div(totalAssets)
		if conservativePct.GreaterThan(insightThresholds.ConservativePct) {
			pct := conservativePct.Mul(decimal.NewFromInt(100)).StringFixed(1)
			insights = append(insights, models.Insight{
				ID:          uuid.New().String(),
				UserID:      userID,
				Category:    models.InsightCategoryRiskTolerance,
				Severity:    models.InsightSeverityInfo,
				Title:       "Portfolio appears overly conservative",
				Body:        fmt.Sprintf("%s%% in cash and bonds. If you have a long horizon, consider growth assets.", pct),
				ActionURL:   "/assets",
				MetadataRaw: fmt.Sprintf(`{"conservative_pct":"%s"}`, pct),
			})
		}
	}

	return insights
}
