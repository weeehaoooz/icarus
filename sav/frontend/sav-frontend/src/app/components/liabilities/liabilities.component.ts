import { Component, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { FinanceService, Liability, LiabilityType } from '../../services/finance.service';

@Component({
  selector: 'app-liabilities',
  imports: [CommonModule, ReactiveFormsModule],
  templateUrl: './liabilities.component.html',
  styleUrl: './liabilities.component.scss'
})
export class LiabilitiesComponent implements OnInit {
  protected readonly finance = inject(FinanceService);
  private readonly fb = inject(FormBuilder);

  // Signals
  protected readonly isAdding = signal(false);
  protected readonly isEditing = signal<string | null>(null);

  // Form setup for creating/editing liability
  protected readonly liabilityForm = this.fb.nonNullable.group({
    name: ['', [Validators.required]],
    liability_type: ['mortgage' as LiabilityType, [Validators.required]],
    principal: ['0', [Validators.required, Validators.pattern(/^[0-9]+(\.[0-9]+)?$/)]],
    outstanding_amt: ['0', [Validators.required, Validators.pattern(/^[0-9]+(\.[0-9]+)?$/)]],
    interest_rate: ['0', [Validators.required, Validators.pattern(/^[0-9]+(\.[0-9]+)?$/)]],
    interest_type: ['fixed' as 'fixed' | 'variable', [Validators.required]],
    currency: ['SGD', [Validators.required]],
    monthly_payment: ['0', [Validators.required, Validators.pattern(/^[0-9]+(\.[0-9]+)?$/)]],
    term_months: [0, [Validators.required, Validators.min(0)]],
    maturity_date: [''],
    lender: [''],
    notes: ['']
  });

  ngOnInit(): void {
    this.finance.loadLiabilities();
  }

  onAddSubmit(): void {
    if (this.liabilityForm.invalid) {
      this.liabilityForm.markAllAsTouched();
      return;
    }
    const val = this.liabilityForm.getRawValue();

    this.finance.createLiability(val).subscribe({
      next: () => {
        this.isAdding.set(false);
        this.liabilityForm.reset();
        this.finance.loadLiabilities();
        this.finance.recompute().subscribe();
      }
    });
  }

  deleteLiability(id: string): void {
    if (confirm('Are you sure you want to delete this liability?')) {
      this.finance.deleteLiability(id).subscribe({
        next: () => {
          this.finance.loadLiabilities();
          this.finance.recompute().subscribe();
        }
      });
    }
  }

  // Quick edit support for Outstanding Amount directly from the list
  startQuickEdit(id: string, currentAmt: string): void {
    this.isEditing.set(id);
  }

  saveQuickEdit(id: string, newAmt: string): void {
    if (isNaN(parseFloat(newAmt)) || parseFloat(newAmt) < 0) {
      alert('Invalid outstanding amount.');
      return;
    }
    this.finance.patchLiability(id, { outstanding_amt: newAmt }).subscribe({
      next: () => {
        this.isEditing.set(null);
        this.finance.loadLiabilities();
        this.finance.recompute().subscribe();
      }
    });
  }

  cancelQuickEdit(): void {
    this.isEditing.set(null);
  }

  formatCurrency(value: string): string {
    const parsed = parseFloat(value) || 0;
    return new Intl.NumberFormat('en-SG', { style: 'currency', currency: 'SGD' }).format(parsed);
  }

  formatPercent(value: string): string {
    const parsed = parseFloat(value) || 0;
    return parsed.toFixed(2) + '%';
  }
}
