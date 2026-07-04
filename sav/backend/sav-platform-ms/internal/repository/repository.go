package repository

import (
	"database/sql"
	"embed"
	"errors"
	"sav-platform-ms/internal/models"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	_ "modernc.org/sqlite"
)

//go:embed migrations
var migrationsFS embed.FS

type SQLRepository struct {
	db *sql.DB
}

func NewSQLRepository(db *sql.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

// InitDB initializes SQLite and runs all up migrations sequentially.
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	_, _ = db.Exec("PRAGMA foreign_keys = ON;")
	_, _ = db.Exec("PRAGMA journal_mode = WAL;")

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		db.Close()
		return nil, err
	}

	entries, err := migrationsFS.ReadDir("migrations/sqlite")
	if err != nil {
		db.Close()
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		version := entry.Name()
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
			db.Close()
			return nil, err
		}
		if count > 0 {
			continue
		}
		upSQL, err := migrationsFS.ReadFile("migrations/sqlite/" + version)
		if err != nil {
			db.Close()
			return nil, err
		}
		for _, stmt := range strings.Split(string(upSQL), ";") {
			trimmed := strings.TrimSpace(stmt)
			if trimmed == "" {
				continue
			}
			if _, err := db.Exec(trimmed); err != nil {
				db.Close()
				return nil, err
			}
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

// ============================================================
// TIME PARSING
// ============================================================

func parseTime(s string) (time.Time, error) {
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	if idx := strings.Index(s, " m="); idx != -1 {
		s = s[:idx]
		for _, l := range layouts {
			if t, err := time.Parse(l, s); err == nil {
				return t, nil
			}
		}
	}
	return time.Time{}, errors.New("failed to parse time: " + s)
}

func decimalFromString(s string) (decimal.Decimal, error) {
	if s == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(s)
}

// ============================================================
// ASSETS
// ============================================================

func (r *SQLRepository) CreateAsset(a *models.Asset) error {
	_, err := r.db.Exec(`
		INSERT INTO finance_assets
		  (id, user_id, name, ticker, isin, asset_class, quantity, unit_cost,
		   current_value, currency, exchange, country, notes, acquired_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.UserID, a.Name, a.Ticker, a.ISIN, string(a.AssetClass),
		a.Quantity.String(), a.UnitCost.String(), a.CurrentValue.String(),
		a.Currency, a.Exchange, a.Country, a.Notes, a.AcquiredAt)
	return err
}

func (r *SQLRepository) ListAssets(userID string) ([]models.Asset, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, name, ticker, isin, asset_class, quantity, unit_cost,
		       current_value, currency, exchange, country, notes, acquired_at,
		       created_at, updated_at
		FROM finance_assets WHERE user_id = ? ORDER BY name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Asset
	for rows.Next() {
		var a models.Asset
		var qty, unitCost, curVal, createdStr, updatedStr, ac string
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Ticker, &a.ISIN, &ac,
			&qty, &unitCost, &curVal, &a.Currency, &a.Exchange, &a.Country,
			&a.Notes, &a.AcquiredAt, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		a.AssetClass = models.AssetClass(ac)
		a.Quantity, _ = decimalFromString(qty)
		a.UnitCost, _ = decimalFromString(unitCost)
		a.CurrentValue, _ = decimalFromString(curVal)
		a.CreatedAt, _ = parseTime(createdStr)
		a.UpdatedAt, _ = parseTime(updatedStr)
		list = append(list, a)
	}
	return list, nil
}

func (r *SQLRepository) GetAsset(id, userID string) (*models.Asset, error) {
	var a models.Asset
	var qty, unitCost, curVal, createdStr, updatedStr, ac string
	err := r.db.QueryRow(`
		SELECT id, user_id, name, ticker, isin, asset_class, quantity, unit_cost,
		       current_value, currency, exchange, country, notes, acquired_at,
		       created_at, updated_at
		FROM finance_assets WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&a.ID, &a.UserID, &a.Name, &a.Ticker, &a.ISIN, &ac,
			&qty, &unitCost, &curVal, &a.Currency, &a.Exchange, &a.Country,
			&a.Notes, &a.AcquiredAt, &createdStr, &updatedStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("asset not found")
		}
		return nil, err
	}
	a.AssetClass = models.AssetClass(ac)
	a.Quantity, _ = decimalFromString(qty)
	a.UnitCost, _ = decimalFromString(unitCost)
	a.CurrentValue, _ = decimalFromString(curVal)
	a.CreatedAt, _ = parseTime(createdStr)
	a.UpdatedAt, _ = parseTime(updatedStr)
	return &a, nil
}

func (r *SQLRepository) UpdateAsset(a *models.Asset) error {
	_, err := r.db.Exec(`
		UPDATE finance_assets SET
		  name=?, ticker=?, isin=?, asset_class=?, quantity=?, unit_cost=?,
		  current_value=?, currency=?, exchange=?, country=?, notes=?,
		  acquired_at=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=? AND user_id=?`,
		a.Name, a.Ticker, a.ISIN, string(a.AssetClass),
		a.Quantity.String(), a.UnitCost.String(), a.CurrentValue.String(),
		a.Currency, a.Exchange, a.Country, a.Notes, a.AcquiredAt,
		a.ID, a.UserID)
	return err
}

func (r *SQLRepository) DeleteAsset(id, userID string) error {
	_, err := r.db.Exec(`DELETE FROM finance_assets WHERE id=? AND user_id=?`, id, userID)
	return err
}

func (r *SQLRepository) BulkUpsertAssets(assets []models.Asset) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, a := range assets {
		_, err = tx.Exec(`
			INSERT INTO finance_assets
			  (id, user_id, name, ticker, isin, asset_class, quantity, unit_cost,
			   current_value, currency, exchange, country, notes, acquired_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
			  name=excluded.name, ticker=excluded.ticker, isin=excluded.isin,
			  asset_class=excluded.asset_class, quantity=excluded.quantity,
			  unit_cost=excluded.unit_cost, current_value=excluded.current_value,
			  currency=excluded.currency, exchange=excluded.exchange,
			  country=excluded.country, notes=excluded.notes,
			  acquired_at=excluded.acquired_at, updated_at=CURRENT_TIMESTAMP`,
			a.ID, a.UserID, a.Name, a.Ticker, a.ISIN, string(a.AssetClass),
			a.Quantity.String(), a.UnitCost.String(), a.CurrentValue.String(),
			a.Currency, a.Exchange, a.Country, a.Notes, a.AcquiredAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ============================================================
// LIABILITIES
// ============================================================

func (r *SQLRepository) CreateLiability(l *models.Liability) error {
	_, err := r.db.Exec(`
		INSERT INTO finance_liabilities
		  (id, user_id, name, liability_type, principal, outstanding_amt, interest_rate,
		   interest_type, currency, monthly_payment, term_months, maturity_date, lender, notes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		l.ID, l.UserID, l.Name, string(l.LiabilityType),
		l.Principal.String(), l.OutstandingAmt.String(), l.InterestRate.String(),
		l.InterestType, l.Currency,
		l.MonthlyPayment.String(), l.TermMonths, l.MaturityDate, l.Lender, l.Notes)
	return err
}

func (r *SQLRepository) ListLiabilities(userID string) ([]models.Liability, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, name, liability_type, principal, outstanding_amt, interest_rate,
		       interest_type, currency, monthly_payment, term_months, maturity_date,
		       lender, notes, created_at, updated_at
		FROM finance_liabilities WHERE user_id=? ORDER BY name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Liability
	for rows.Next() {
		var l models.Liability
		var lt, prin, outAmt, iRate, mPay, createdStr, updatedStr string
		if err := rows.Scan(&l.ID, &l.UserID, &l.Name, &lt, &prin, &outAmt, &iRate,
			&l.InterestType, &l.Currency, &mPay, &l.TermMonths, &l.MaturityDate,
			&l.Lender, &l.Notes, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		l.LiabilityType = models.LiabilityType(lt)
		l.Principal, _ = decimalFromString(prin)
		l.OutstandingAmt, _ = decimalFromString(outAmt)
		l.InterestRate, _ = decimalFromString(iRate)
		l.MonthlyPayment, _ = decimalFromString(mPay)
		l.CreatedAt, _ = parseTime(createdStr)
		l.UpdatedAt, _ = parseTime(updatedStr)
		list = append(list, l)
	}
	return list, nil
}

func (r *SQLRepository) UpdateLiability(l *models.Liability) error {
	_, err := r.db.Exec(`
		UPDATE finance_liabilities SET
		  name=?, liability_type=?, principal=?, outstanding_amt=?, interest_rate=?,
		  interest_type=?, currency=?, monthly_payment=?, term_months=?,
		  maturity_date=?, lender=?, notes=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=? AND user_id=?`,
		l.Name, string(l.LiabilityType),
		l.Principal.String(), l.OutstandingAmt.String(), l.InterestRate.String(),
		l.InterestType, l.Currency,
		l.MonthlyPayment.String(), l.TermMonths, l.MaturityDate, l.Lender, l.Notes,
		l.ID, l.UserID)
	return err
}

func (r *SQLRepository) DeleteLiability(id, userID string) error {
	_, err := r.db.Exec(`DELETE FROM finance_liabilities WHERE id=? AND user_id=?`, id, userID)
	return err
}

// ============================================================
// INSIGHTS
// ============================================================

func (r *SQLRepository) UpsertInsights(userID string, insights []models.Insight) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`DELETE FROM finance_insights WHERE user_id=? AND is_dismissed=0`, userID); err != nil {
		return err
	}
	for _, ins := range insights {
		if _, err = tx.Exec(`
			INSERT INTO finance_insights
			  (id, user_id, category, severity, title, body, action_url, metadata, computed_at)
			VALUES (?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
			ins.ID, ins.UserID, string(ins.Category), string(ins.Severity),
			ins.Title, ins.Body, ins.ActionURL, ins.MetadataRaw); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SQLRepository) ListInsights(userID string) ([]models.Insight, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, category, severity, title, body, action_url,
		       is_read, is_dismissed, computed_at
		FROM finance_insights WHERE user_id=? AND is_dismissed=0
		ORDER BY computed_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Insight
	for rows.Next() {
		var ins models.Insight
		var cat, sev, compStr string
		if err := rows.Scan(&ins.ID, &ins.UserID, &cat, &sev, &ins.Title, &ins.Body,
			&ins.ActionURL, &ins.IsRead, &ins.IsDismissed, &compStr); err != nil {
			return nil, err
		}
		ins.Category = models.InsightCategory(cat)
		ins.Severity = models.InsightSeverity(sev)
		ins.ComputedAt, _ = parseTime(compStr)
		list = append(list, ins)
	}
	return list, nil
}

func (r *SQLRepository) MarkInsightRead(id, userID string) error {
	_, err := r.db.Exec(`UPDATE finance_insights SET is_read=1 WHERE id=? AND user_id=?`, id, userID)
	return err
}

func (r *SQLRepository) DismissInsight(id, userID string) error {
	_, err := r.db.Exec(`UPDATE finance_insights SET is_dismissed=1 WHERE id=? AND user_id=?`, id, userID)
	return err
}

func (r *SQLRepository) CountUnreadInsights(userID string) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM finance_insights WHERE user_id=? AND is_read=0 AND is_dismissed=0`, userID).
		Scan(&count)
	return count, err
}

// GetBaseCurrency retrieves the user's preferred reporting currency, defaulting to "SGD".
func (r *SQLRepository) GetBaseCurrency(userID string) (string, error) {
	var currency string
	err := r.db.QueryRow(`SELECT base_currency FROM user_preferences WHERE user_id = ?`, userID).Scan(&currency)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no such table") {
			return "SGD", nil
		}
		return "", err
	}
	return currency, nil
}

// UpdateBaseCurrency inserts or updates the user's preferred reporting currency.
func (r *SQLRepository) UpdateBaseCurrency(userID string, baseCurrency string) error {
	_, err := r.db.Exec(`
		INSERT INTO user_preferences (user_id, base_currency, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET base_currency = excluded.base_currency, updated_at = CURRENT_TIMESTAMP`,
		userID, baseCurrency)
	return err
}

