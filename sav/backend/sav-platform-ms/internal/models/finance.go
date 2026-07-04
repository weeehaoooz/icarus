package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// AssetClass categorises an asset for insight computation and allocation reporting.
type AssetClass string

const (
	AssetClassCash        AssetClass = "cash"
	AssetClassEquity      AssetClass = "equity"
	AssetClassBond        AssetClass = "bond"
	AssetClassRealEstate  AssetClass = "real_estate"
	AssetClassCrypto      AssetClass = "crypto"
	AssetClassCommodity   AssetClass = "commodity"
	AssetClassAlternative AssetClass = "alternative"
)

// Asset represents a single financial asset owned by a user.
type Asset struct {
	ID           string          `json:"id"            db:"id"`
	UserID       string          `json:"user_id"       db:"user_id"`
	Name         string          `json:"name"          db:"name"`
	Ticker       string          `json:"ticker"        db:"ticker"`
	ISIN         string          `json:"isin"          db:"isin"`
	AssetClass   AssetClass      `json:"asset_class"   db:"asset_class"`
	Quantity     decimal.Decimal `json:"quantity"      db:"quantity"`
	UnitCost     decimal.Decimal `json:"unit_cost"     db:"unit_cost"`
	CurrentValue decimal.Decimal `json:"current_value" db:"current_value"`
	LivePrice    decimal.Decimal `json:"live_price,omitempty"` // enriched at read time, not persisted
	Currency     string          `json:"currency"      db:"currency"`
	Exchange     string          `json:"exchange"      db:"exchange"`
	Country      string          `json:"country"       db:"country"`
	Notes        string          `json:"notes"         db:"notes"`
	AcquiredAt   string          `json:"acquired_at"   db:"acquired_at"`
	CreatedAt    time.Time       `json:"created_at"    db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"    db:"updated_at"`
}

// AssetImportRow is the flat schema for CSV bulk import.
type AssetImportRow struct {
	Name       string `csv:"name"`
	Ticker     string `csv:"ticker"`
	ISIN       string `csv:"isin"`
	AssetClass string `csv:"asset_class"`
	Quantity   string `csv:"quantity"`
	UnitCost   string `csv:"unit_cost"`
	Currency   string `csv:"currency"`
	Exchange   string `csv:"exchange"`
	Country    string `csv:"country"`
	Notes      string `csv:"notes"`
	AcquiredAt string `csv:"acquired_at"`
}

// ImportResult is returned after a bulk CSV import.
type ImportResult struct {
	Imported int           `json:"imported"`
	Skipped  int           `json:"skipped"`
	Errors   []ImportError `json:"errors,omitempty"`
}

// ImportError identifies a validation failure on a specific CSV row.
type ImportError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// LiabilityType categorises a debt instrument.
type LiabilityType string

const (
	LiabilityTypeMortgage   LiabilityType = "mortgage"
	LiabilityTypePersonal   LiabilityType = "personal_loan"
	LiabilityTypeAuto       LiabilityType = "auto_loan"
	LiabilityTypeStudy      LiabilityType = "student_loan"
	LiabilityTypeCreditCard LiabilityType = "credit_card"
	LiabilityTypeMargin     LiabilityType = "margin_loan"
	LiabilityTypeOther      LiabilityType = "other"
)

// Liability represents a financial obligation owed by the user.
type Liability struct {
	ID             string          `json:"id"              db:"id"`
	UserID         string          `json:"user_id"         db:"user_id"`
	Name           string          `json:"name"            db:"name"`
	LiabilityType  LiabilityType   `json:"liability_type"  db:"liability_type"`
	Principal      decimal.Decimal `json:"principal"       db:"principal"`
	OutstandingAmt decimal.Decimal `json:"outstanding_amt" db:"outstanding_amt"`
	InterestRate   decimal.Decimal `json:"interest_rate"   db:"interest_rate"`
	InterestType   string          `json:"interest_type"   db:"interest_type"`
	Currency       string          `json:"currency"        db:"currency"`
	MonthlyPayment decimal.Decimal `json:"monthly_payment" db:"monthly_payment"`
	TermMonths     int             `json:"term_months"     db:"term_months"`
	MaturityDate   string          `json:"maturity_date"   db:"maturity_date"`
	Lender         string          `json:"lender"          db:"lender"`
	Notes          string          `json:"notes"           db:"notes"`
	CreatedAt      time.Time       `json:"created_at"      db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"      db:"updated_at"`
}

// InsightSeverity controls UI urgency.
type InsightSeverity string

const (
	InsightSeverityInfo     InsightSeverity = "info"
	InsightSeverityWarning  InsightSeverity = "warning"
	InsightSeverityCritical InsightSeverity = "critical"
)

// InsightCategory maps to a UI section/filter.
type InsightCategory string

const (
	InsightCategoryIdleCash      InsightCategory = "idle_cash"
	InsightCategoryConcentration InsightCategory = "concentration_risk"
	InsightCategoryLeverage      InsightCategory = "leverage"
	InsightCategoryRiskTolerance InsightCategory = "risk_tolerance"
	InsightCategoryDebtOptimise  InsightCategory = "debt_optimisation"
)

// Insight is a single computed observation about a user's financial position.
type Insight struct {
	ID          string          `json:"id"           db:"id"`
	UserID      string          `json:"user_id"      db:"user_id"`
	Category    InsightCategory `json:"category"     db:"category"`
	Severity    InsightSeverity `json:"severity"     db:"severity"`
	Title       string          `json:"title"        db:"title"`
	Body        string          `json:"body"         db:"body"`
	ActionURL   string          `json:"action_url"   db:"action_url"`
	MetadataRaw string          `json:"-"            db:"metadata"`
	IsRead      bool            `json:"is_read"      db:"is_read"`
	IsDismissed bool            `json:"is_dismissed" db:"is_dismissed"`
	ComputedAt  time.Time       `json:"computed_at"  db:"computed_at"`
}

// AllocationSlice is one slice of the portfolio breakdown.
type AllocationSlice struct {
	AssetClass AssetClass      `json:"asset_class"`
	Value      decimal.Decimal `json:"value"`
	Pct        decimal.Decimal `json:"pct"`
}

// FinancialSummary is the top-level portfolio snapshot returned by GET /api/v1/finance/summary.
type FinancialSummary struct {
	UserID           string            `json:"user_id"`
	TotalAssets      decimal.Decimal   `json:"total_assets"`
	TotalLiabilities decimal.Decimal   `json:"total_liabilities"`
	NetWorth         decimal.Decimal   `json:"net_worth"`
	BaseCurrency     string            `json:"base_currency"`
	Allocation       []AllocationSlice `json:"allocation"`
	DebtToEquity     decimal.Decimal   `json:"debt_to_equity_ratio"`
	CashPct          decimal.Decimal   `json:"cash_pct"`
	PendingInsights  int               `json:"pending_insights"`
	AsOf             time.Time         `json:"as_of"`
}
