package handler

import (
	"encoding/json"
	"sav-platform-ms/internal/models"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ============================================================
// AUTH MIDDLEWARE
// ============================================================

// AuthRequired validates any JWT (no role check). Injects user_id into X-User-ID header.
func (s *HandlerServer) AuthRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.respondWithError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			s.respondWithError(w, http.StatusUnauthorized, "invalid authorization format")
			return
		}
		claims, err := s.Verifier.VerifyToken(parts[1])
		if err != nil {
			s.respondWithError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}
		r.Header.Set("X-User-ID", claims.Subject)
		next(w, r)
	}
}

func userIDFromRequest(r *http.Request) string {
	return r.Header.Get("X-User-ID")
}

// ============================================================
// SUMMARY
// ============================================================

func (s *HandlerServer) FinanceGetSummaryHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	assets, err := s.Repo.ListAssets(userID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	liabilities, err := s.Repo.ListLiabilities(userID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	pending, _ := s.Repo.CountUnreadInsights(userID)

	baseCurrency, err := s.Repo.GetBaseCurrency(userID)
	if err != nil {
		baseCurrency = "SGD"
	}

	summary := buildSummary(userID, assets, liabilities, pending, baseCurrency)
	s.respondWithJSON(w, http.StatusOK, summary)
}

func buildSummary(userID string, assets []models.Asset, liabilities []models.Liability, pendingInsights int, baseCurrency string) models.FinancialSummary {
	totalAssets := decimal.Zero
	totalLiabilities := decimal.Zero
	byClass := map[models.AssetClass]decimal.Decimal{}

	for _, a := range assets {
		totalAssets = totalAssets.Add(a.CurrentValue)
		byClass[a.AssetClass] = byClass[a.AssetClass].Add(a.CurrentValue)
	}
	for _, l := range liabilities {
		totalLiabilities = totalLiabilities.Add(l.OutstandingAmt)
	}

	var allocation []models.AllocationSlice
	if totalAssets.IsPositive() {
		for ac, val := range byClass {
			pct := val.Div(totalAssets).Mul(decimal.NewFromInt(100))
			allocation = append(allocation, models.AllocationSlice{AssetClass: ac, Value: val, Pct: pct})
		}
	}

	cashVal := byClass[models.AssetClassCash]
	cashPct := decimal.Zero
	if totalAssets.IsPositive() {
		cashPct = cashVal.Div(totalAssets).Mul(decimal.NewFromInt(100))
	}
	debtToEquity := decimal.Zero
	if totalAssets.IsPositive() {
		debtToEquity = totalLiabilities.Div(totalAssets)
	}

	return models.FinancialSummary{
		UserID:           userID,
		TotalAssets:      totalAssets,
		TotalLiabilities: totalLiabilities,
		NetWorth:         totalAssets.Sub(totalLiabilities),
		BaseCurrency:     baseCurrency,
		Allocation:       allocation,
		DebtToEquity:     debtToEquity,
		CashPct:          cashPct,
		PendingInsights:  pendingInsights,
		AsOf:             time.Now(),
	}
}

// ============================================================
// ASSETS
// ============================================================

func (s *HandlerServer) FinanceListAssetsHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	assets, err := s.Repo.ListAssets(userID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Enrich equity assets with live Yahoo Finance prices
	if assets != nil {
		var tickers []string
		tickerIdx := map[string]int{}
		for i, a := range assets {
			if a.AssetClass == models.AssetClassEquity && a.Ticker != "" {
				tickers = append(tickers, a.Ticker)
				tickerIdx[a.Ticker] = i
			}
		}
		if len(tickers) > 0 {
			prices := fetchYahooPrices(tickers)
			for ticker, price := range prices {
				if idx, ok := tickerIdx[ticker]; ok {
					assets[idx].LivePrice = price
				}
			}
		}
	}
	if assets == nil {
		assets = []models.Asset{}
	}
	s.respondWithJSON(w, http.StatusOK, assets)
}

func (s *HandlerServer) FinanceCreateAssetHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	var req models.Asset
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		s.respondWithError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Currency == "" {
		req.Currency = "SGD"
	}

	req.ID = uuid.New().String()
	req.UserID = userID
	req.CreatedAt = time.Now()
	req.UpdatedAt = time.Now()

	if err := s.Repo.CreateAsset(&req); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusCreated, req)
}

func (s *HandlerServer) FinancePatchAssetHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	id := r.PathValue("id")
	if id == "" {
		s.respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}

	existing, err := s.Repo.GetAsset(id, userID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if v, ok := patch["name"].(string); ok {
		existing.Name = v
	}
	if v, ok := patch["ticker"].(string); ok {
		existing.Ticker = v
	}
	if v, ok := patch["isin"].(string); ok {
		existing.ISIN = v
	}
	if v, ok := patch["asset_class"].(string); ok {
		existing.AssetClass = models.AssetClass(v)
	}
	if v, ok := patch["quantity"].(string); ok {
		existing.Quantity, _ = decimal.NewFromString(v)
	}
	if v, ok := patch["unit_cost"].(string); ok {
		existing.UnitCost, _ = decimal.NewFromString(v)
	}
	if v, ok := patch["current_value"].(string); ok {
		existing.CurrentValue, _ = decimal.NewFromString(v)
	}
	if v, ok := patch["currency"].(string); ok {
		existing.Currency = v
	}
	if v, ok := patch["notes"].(string); ok {
		existing.Notes = v
	}
	if v, ok := patch["acquired_at"].(string); ok {
		existing.AcquiredAt = v
	}

	if err := s.Repo.UpdateAsset(existing); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, existing)
}

func (s *HandlerServer) FinanceDeleteAssetHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	id := r.PathValue("id")
	if err := s.Repo.DeleteAsset(id, userID); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HandlerServer) FinanceImportAssetsHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		s.respondWithError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	var rows []models.AssetImportRow
	if err := gocsv.Unmarshal(file, &rows); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "failed to parse CSV: "+err.Error())
		return
	}

	result := models.ImportResult{}
	var toInsert []models.Asset

	for i, row := range rows {
		if strings.TrimSpace(row.Name) == "" {
			result.Errors = append(result.Errors, models.ImportError{Row: i + 2, Field: "name", Message: "required"})
			result.Skipped++
			continue
		}
		qty, err := decimal.NewFromString(row.Quantity)
		if err != nil || qty.IsNegative() {
			result.Errors = append(result.Errors, models.ImportError{Row: i + 2, Field: "quantity", Message: "invalid"})
			result.Skipped++
			continue
		}
		unitCost, err := decimal.NewFromString(row.UnitCost)
		if err != nil || unitCost.IsNegative() {
			result.Errors = append(result.Errors, models.ImportError{Row: i + 2, Field: "unit_cost", Message: "invalid"})
			result.Skipped++
			continue
		}
		currency := strings.ToUpper(row.Currency)
		if len(currency) != 3 {
			currency = "SGD"
		}
		toInsert = append(toInsert, models.Asset{
			ID:           uuid.New().String(),
			UserID:       userID,
			Name:         strings.TrimSpace(row.Name),
			Ticker:       strings.ToUpper(strings.TrimSpace(row.Ticker)),
			ISIN:         strings.TrimSpace(row.ISIN),
			AssetClass:   models.AssetClass(strings.ToLower(strings.TrimSpace(row.AssetClass))),
			Quantity:     qty,
			UnitCost:     unitCost,
			CurrentValue: qty.Mul(unitCost),
			Currency:     currency,
			Exchange:     strings.TrimSpace(row.Exchange),
			Country:      strings.TrimSpace(row.Country),
			Notes:        strings.TrimSpace(row.Notes),
			AcquiredAt:   strings.TrimSpace(row.AcquiredAt),
		})
		result.Imported++
	}

	if len(toInsert) > 0 {
		if err := s.Repo.BulkUpsertAssets(toInsert); err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "import failed: "+err.Error())
			return
		}
	}
	s.respondWithJSON(w, http.StatusOK, result)
}

// ============================================================
// LIABILITIES
// ============================================================

func (s *HandlerServer) FinanceListLiabilitiesHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	list, err := s.Repo.ListLiabilities(userID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []models.Liability{}
	}
	s.respondWithJSON(w, http.StatusOK, list)
}

func (s *HandlerServer) FinanceCreateLiabilityHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	var req models.Liability
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		s.respondWithError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Currency == "" {
		req.Currency = "SGD"
	}
	if req.InterestType == "" {
		req.InterestType = "fixed"
	}

	req.ID = uuid.New().String()
	req.UserID = userID
	req.CreatedAt = time.Now()
	req.UpdatedAt = time.Now()

	if err := s.Repo.CreateLiability(&req); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusCreated, req)
}

func (s *HandlerServer) FinancePatchLiabilityHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	id := r.PathValue("id")

	liabilities, err := s.Repo.ListLiabilities(userID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var existing *models.Liability
	for i := range liabilities {
		if liabilities[i].ID == id {
			existing = &liabilities[i]
			break
		}
	}
	if existing == nil {
		s.respondWithError(w, http.StatusNotFound, "liability not found")
		return
	}

	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if v, ok := patch["name"].(string); ok {
		existing.Name = v
	}
	if v, ok := patch["liability_type"].(string); ok {
		existing.LiabilityType = models.LiabilityType(v)
	}
	if v, ok := patch["outstanding_amt"].(string); ok {
		existing.OutstandingAmt, _ = decimal.NewFromString(v)
	}
	if v, ok := patch["interest_rate"].(string); ok {
		existing.InterestRate, _ = decimal.NewFromString(v)
	}
	if v, ok := patch["monthly_payment"].(string); ok {
		existing.MonthlyPayment, _ = decimal.NewFromString(v)
	}
	if v, ok := patch["notes"].(string); ok {
		existing.Notes = v
	}

	if err := s.Repo.UpdateLiability(existing); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, existing)
}

func (s *HandlerServer) FinanceDeleteLiabilityHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	id := r.PathValue("id")
	if err := s.Repo.DeleteLiability(id, userID); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// INSIGHTS
// ============================================================

func (s *HandlerServer) FinanceListInsightsHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	list, err := s.Repo.ListInsights(userID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []models.Insight{}
	}
	s.respondWithJSON(w, http.StatusOK, list)
}

func (s *HandlerServer) FinanceMarkInsightReadHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	id := r.PathValue("id")
	if err := s.Repo.MarkInsightRead(id, userID); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HandlerServer) FinanceDismissInsightHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	id := r.PathValue("id")
	if err := s.Repo.DismissInsight(id, userID); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *HandlerServer) FinanceRecomputeInsightsHandler(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	assets, err := s.Repo.ListAssets(userID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	liabilities, err := s.Repo.ListLiabilities(userID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	insights := ComputeInsights(userID, assets, liabilities)
	if err := s.Repo.UpsertInsights(userID, insights); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "recomputed",
		"count":   len(insights),
		"user_id": userID,
	})
}

// ============================================================
// LIVE PRICES — Yahoo Finance (free, no API key)
// ============================================================

type yahooQuoteResponse struct {
	QuoteResponse struct {
		Result []struct {
			Symbol             string  `json:"symbol"`
			RegularMarketPrice float64 `json:"regularMarketPrice"`
		} `json:"result"`
	} `json:"quoteResponse"`
}

func fetchYahooPrices(tickers []string) map[string]decimal.Decimal {
	result := map[string]decimal.Decimal{}
	if len(tickers) == 0 {
		return result
	}
	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v7/finance/quote?symbols=%s&fields=regularMarketPrice",
		strings.Join(tickers, ","))

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Printf("[Finance] price fetch build error: %v", err)
		return result
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[Finance] price fetch error: %v", err)
		return result
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result
	}

	var data yahooQuoteResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return result
	}
	for _, q := range data.QuoteResponse.Result {
		if q.RegularMarketPrice > 0 {
			result[q.Symbol] = decimal.NewFromFloat(q.RegularMarketPrice)
		}
	}
	return result
}

func (s *HandlerServer) FinanceGetPricesHandler(w http.ResponseWriter, r *http.Request) {
	tickersParam := r.URL.Query().Get("tickers")
	if tickersParam == "" {
		s.respondWithError(w, http.StatusBadRequest, "tickers query param is required")
		return
	}
	tickers := strings.Split(tickersParam, ",")
	prices := fetchYahooPrices(tickers)
	out := map[string]string{}
	for k, v := range prices {
		out[k] = v.String()
	}
	priceBytes, _ := json.Marshal(out)
	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{"prices": json.RawMessage(priceBytes)})
}

type UpdatePreferencesRequest struct {
	BaseCurrency string `json:"base_currency"`
}

func (s *HandlerServer) FinanceGetPreferencesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := userIDFromRequest(r)
	currency, err := s.Repo.GetBaseCurrency(userID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to get preferences: "+err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, map[string]string{"base_currency": currency})
}

func (s *HandlerServer) FinanceUpdatePreferencesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BaseCurrency == "" {
		s.respondWithError(w, http.StatusBadRequest, "base_currency is required")
		return
	}

	req.BaseCurrency = strings.ToUpper(strings.TrimSpace(req.BaseCurrency))
	if len(req.BaseCurrency) != 3 {
		s.respondWithError(w, http.StatusBadRequest, "invalid base_currency format (must be 3 characters, e.g. USD, SGD)")
		return
	}

	userID := userIDFromRequest(r)
	if err := s.Repo.UpdateBaseCurrency(userID, req.BaseCurrency); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to update preferences: "+err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"message":       "preferences updated successfully",
		"base_currency": req.BaseCurrency,
	})
}
