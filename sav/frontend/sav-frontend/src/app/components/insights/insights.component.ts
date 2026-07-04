import { Component, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { FinanceService, Insight } from '../../services/finance.service';

@Component({
  selector: 'app-insights',
  imports: [CommonModule, RouterLink],
  templateUrl: './insights.component.html',
  styleUrl: './insights.component.scss'
})
export class InsightsComponent implements OnInit {
  protected readonly finance = inject(FinanceService);

  // Signals
  protected readonly recomputing = signal(false);

  ngOnInit(): void {
    this.finance.loadInsights();
  }

  markAsRead(id: string): void {
    this.finance.markRead(id).subscribe({
      next: () => this.finance.loadInsights()
    });
  }

  dismiss(id: string): void {
    this.finance.dismiss(id).subscribe({
      next: () => this.finance.loadInsights()
    });
  }

  triggerRecompute(): void {
    this.recomputing.set(true);
    this.finance.recompute().subscribe({
      next: () => {
        this.recomputing.set(false);
        this.finance.loadInsights();
      },
      error: () => {
        this.recomputing.set(false);
      }
    });
  }

  formatCategory(cat: string): string {
    if (!cat) return '';
    return cat.split('_')
      .map(word => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ');
  }
}
