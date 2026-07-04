import { Component, inject, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';
import { CommonModule } from '@angular/common';
import { FinanceService } from '../../services/finance.service';

@Component({
  selector: 'app-dashboard',
  imports: [CommonModule, RouterLink],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.scss'
})
export class DashboardComponent implements OnInit {
  protected readonly finance = inject(FinanceService);
  protected readonly parseFloat = parseFloat;

  ngOnInit(): void {
    this.finance.loadSummary();
    this.finance.loadInsights();
  }

  // Format currency values safely.
  formatCurrency(value: string | undefined): string {
    const summary = this.finance.summary();
    const currency = summary?.base_currency || 'SGD';

    if (!value) return `${currency} 0.00`;
    const parsed = parseFloat(value);
    if (isNaN(parsed)) return `${currency} 0.00`;
    
    try {
      return new Intl.NumberFormat('en-SG', { style: 'currency', currency: currency }).format(parsed);
    } catch {
      return `${currency} ${parsed.toFixed(2)}`;
    }
  }

  // Format percentages.
  formatPercent(value: string | undefined): string {
    if (!value) return '0.0%';
    const parsed = parseFloat(value);
    if (isNaN(parsed)) return '0.0%';
    return parsed.toFixed(1) + '%';
  }

  // Format float ratio numbers.
  formatRatio(value: string | undefined): string {
    if (!value) return '0.00';
    const parsed = parseFloat(value);
    if (isNaN(parsed)) return '0.00';
    return parsed.toFixed(2);
  }
}
