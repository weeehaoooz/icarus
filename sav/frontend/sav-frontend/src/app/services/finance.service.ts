import { Injectable, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

// ── Data Types ───────────────────────────────────────────────
export type AssetClass =
  | 'cash' | 'equity' | 'bond' | 'real_estate' | 'crypto' | 'commodity' | 'alternative';

export type LiabilityType =
  | 'mortgage' | 'personal_loan' | 'auto_loan' | 'student_loan' | 'credit_card' | 'margin_loan' | 'other';

export type InsightSeverity = 'info' | 'warning' | 'critical';
export type InsightCategory =
  | 'idle_cash' | 'concentration_risk' | 'leverage' | 'risk_tolerance' | 'debt_optimisation';

export interface Asset {
  id: string;
  user_id: string;
  name: string;
  ticker: string;
  isin: string;
  asset_class: AssetClass;
  quantity: string;
  unit_cost: string;
  current_value: string;
  live_price?: string;
  currency: string;
  exchange: string;
  country: string;
  notes: string;
  acquired_at: string;
  created_at: string;
  updated_at: string;
}

export interface Liability {
  id: string;
  user_id: string;
  name: string;
  liability_type: LiabilityType;
  principal: string;
  outstanding_amt: string;
  interest_rate: string;
  interest_type: 'fixed' | 'variable';
  currency: string;
  monthly_payment: string;
  term_months: number;
  maturity_date: string;
  lender: string;
  notes: string;
  created_at: string;
  updated_at: string;
}

export interface Insight {
  id: string;
  user_id: string;
  category: InsightCategory;
  severity: InsightSeverity;
  title: string;
  body: string;
  action_url: string;
  is_read: boolean;
  is_dismissed: boolean;
  computed_at: string;
}

export interface AllocationSlice {
  asset_class: AssetClass;
  value: string;
  pct: string;
}

export interface FinancialSummary {
  user_id: string;
  total_assets: string;
  total_liabilities: string;
  net_worth: string;
  base_currency: string;
  allocation: AllocationSlice[];
  debt_to_equity_ratio: string;
  cash_pct: string;
  pending_insights: number;
  as_of: string;
}

export interface ImportResult {
  imported: number;
  skipped: number;
  errors?: { row: number; field: string; message: string }[];
}

// ── Service ──────────────────────────────────────────────────
@Injectable({ providedIn: 'root' })
export class FinanceService {
  private readonly http = inject(HttpClient);
  private readonly base = 'http://localhost:8081/api/v1/finance';

  // Reactive signals — components read these directly
  readonly summary = signal<FinancialSummary | null>(null);
  readonly assets = signal<Asset[]>([]);
  readonly liabilities = signal<Liability[]>([]);
  readonly insights = signal<Insight[]>([]);
  readonly loading = signal(false);

  // ── Summary ─────────────────────────────────────────────
  getSummary(): Observable<FinancialSummary> {
    return this.http.get<FinancialSummary>(`${this.base}/summary`);
  }

  loadSummary(): void {
    this.loading.set(true);
    this.getSummary().subscribe({
      next: s => { this.summary.set(s); this.loading.set(false); },
      error: () => this.loading.set(false)
    });
  }

  // ── Assets ──────────────────────────────────────────────
  listAssets(): Observable<Asset[]> {
    return this.http.get<Asset[]>(`${this.base}/assets`);
  }

  loadAssets(): void {
    this.loading.set(true);
    this.listAssets().subscribe({
      next: list => { this.assets.set(list); this.loading.set(false); },
      error: () => this.loading.set(false)
    });
  }

  createAsset(asset: Partial<Asset>): Observable<Asset> {
    return this.http.post<Asset>(`${this.base}/assets`, asset);
  }

  patchAsset(id: string, patch: Partial<Asset>): Observable<Asset> {
    return this.http.patch<Asset>(`${this.base}/assets/${id}`, patch);
  }

  deleteAsset(id: string): Observable<void> {
    return this.http.delete<void>(`${this.base}/assets/${id}`);
  }

  importAssets(file: File): Observable<ImportResult> {
    const form = new FormData();
    form.append('file', file);
    return this.http.post<ImportResult>(`${this.base}/assets/import`, form);
  }

  // ── Liabilities ─────────────────────────────────────────
  listLiabilities(): Observable<Liability[]> {
    return this.http.get<Liability[]>(`${this.base}/liabilities`);
  }

  loadLiabilities(): void {
    this.loading.set(true);
    this.listLiabilities().subscribe({
      next: list => { this.liabilities.set(list); this.loading.set(false); },
      error: () => this.loading.set(false)
    });
  }

  createLiability(l: Partial<Liability>): Observable<Liability> {
    return this.http.post<Liability>(`${this.base}/liabilities`, l);
  }

  patchLiability(id: string, patch: Partial<Liability>): Observable<Liability> {
    return this.http.patch<Liability>(`${this.base}/liabilities/${id}`, patch);
  }

  deleteLiability(id: string): Observable<void> {
    return this.http.delete<void>(`${this.base}/liabilities/${id}`);
  }

  // ── Insights ────────────────────────────────────────────
  listInsights(): Observable<Insight[]> {
    return this.http.get<Insight[]>(`${this.base}/insights`);
  }

  loadInsights(): void {
    this.listInsights().subscribe({ next: list => this.insights.set(list) });
  }

  markRead(id: string): Observable<void> {
    return this.http.patch<void>(`${this.base}/insights/${id}/read`, {});
  }

  dismiss(id: string): Observable<void> {
    return this.http.patch<void>(`${this.base}/insights/${id}/dismiss`, {});
  }

  recompute(): Observable<{ status: string; count: number }> {
    return this.http.post<{ status: string; count: number }>(`${this.base}/insights/recompute`, {});
  }

  // ── Live Prices ─────────────────────────────────────────
  getPrices(tickers: string[]): Observable<{ prices: Record<string, string> }> {
    return this.http.get<{ prices: Record<string, string> }>(
      `${this.base}/prices?tickers=${tickers.join(',')}`
    );
  }

  // ── Preferences ─────────────────────────────────────────
  getPreferences(): Observable<{ base_currency: string }> {
    return this.http.get<{ base_currency: string }>(`${this.base}/preferences`);
  }

  updatePreferences(baseCurrency: string): Observable<{ message: string; base_currency: string }> {
    return this.http.put<{ message: string; base_currency: string }>(`${this.base}/preferences`, {
      base_currency: baseCurrency
    });
  }
}
