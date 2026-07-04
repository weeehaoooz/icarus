import { Component, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { AuthService } from '../../services/auth.service';
import { FinanceService } from '../../services/finance.service';

@Component({
  selector: 'app-settings',
  imports: [CommonModule, ReactiveFormsModule],
  templateUrl: './settings.component.html',
  styleUrl: './settings.component.scss'
})
export class SettingsComponent implements OnInit {
  protected readonly auth = inject(AuthService);
  protected readonly finance = inject(FinanceService);
  private readonly fb = inject(FormBuilder);

  // Loading & Alert signals
  protected readonly loadingProfile = signal(false);
  protected readonly loadingPassword = signal(false);
  protected readonly loadingCurrency = signal(false);

  protected readonly profileSuccess = signal<string | null>(null);
  protected readonly profileError = signal<string | null>(null);

  protected readonly passwordSuccess = signal<string | null>(null);
  protected readonly passwordError = signal<string | null>(null);

  protected readonly currencySuccess = signal<string | null>(null);
  protected readonly currencyError = signal<string | null>(null);

  // Available reporting currencies
  protected readonly currencies = [
    { code: 'SGD', name: 'Singapore Dollar (SGD)' },
    { code: 'USD', name: 'United States Dollar (USD)' },
    { code: 'EUR', name: 'Euro (EUR)' },
    { code: 'GBP', name: 'British Pound (GBP)' },
    { code: 'AUD', name: 'Australian Dollar (AUD)' },
    { code: 'CNY', name: 'Chinese Yuan (CNY)' },
    { code: 'JPY', name: 'Japanese Yen (JPY)' }
  ];

  // Forms
  protected readonly profileForm = this.fb.nonNullable.group({
    username: [{ value: '', disabled: true }],
    email: ['', [Validators.required, Validators.email]],
    first_name: [''],
    last_name: ['']
  });

  protected readonly passwordForm = this.fb.nonNullable.group({
    old_password: ['', [Validators.required]],
    new_password: ['', [Validators.required, Validators.minLength(6)]],
    confirm_password: ['', [Validators.required]]
  });

  protected readonly currencyForm = this.fb.nonNullable.group({
    base_currency: ['SGD', [Validators.required]]
  });

  ngOnInit(): void {
    this.loadUserProfile();
    this.loadUserPreferences();
  }

  private loadUserProfile(): void {
    this.auth.getProfile().subscribe({
      next: (user) => {
        this.profileForm.patchValue({
          username: user.username,
          email: user.email,
          first_name: user.first_name,
          last_name: user.last_name
        });
      },
      error: (err) => {
        this.profileError.set('Failed to load user profile');
      }
    });
  }

  private loadUserPreferences(): void {
    this.finance.getPreferences().subscribe({
      next: (prefs) => {
        this.currencyForm.patchValue({
          base_currency: prefs.base_currency
        });
      },
      error: () => {
        this.currencyError.set('Failed to load currency preferences');
      }
    });
  }

  onUpdateProfile(): void {
    if (this.profileForm.invalid) {
      this.profileForm.markAllAsTouched();
      return;
    }

    this.loadingProfile.set(true);
    this.profileSuccess.set(null);
    this.profileError.set(null);

    const { email, first_name, last_name } = this.profileForm.getRawValue();

    this.auth.updateProfile({ email, first_name, last_name }).subscribe({
      next: () => {
        this.loadingProfile.set(false);
        this.profileSuccess.set('Profile updated successfully.');
        setTimeout(() => this.profileSuccess.set(null), 5000);
      },
      error: (err) => {
        this.loadingProfile.set(false);
        this.profileError.set(err.error?.error || 'Failed to update profile.');
      }
    });
  }

  onChangePassword(): void {
    if (this.passwordForm.invalid) {
      this.passwordForm.markAllAsTouched();
      return;
    }

    const { old_password, new_password, confirm_password } = this.passwordForm.getRawValue();

    if (new_password !== confirm_password) {
      this.passwordError.set('New passwords do not match.');
      return;
    }

    this.loadingPassword.set(true);
    this.passwordSuccess.set(null);
    this.passwordError.set(null);

    this.auth.changePassword({ old_password, new_password }).subscribe({
      next: () => {
        this.loadingPassword.set(false);
        this.passwordSuccess.set('Password changed successfully.');
        this.passwordForm.reset();
        setTimeout(() => this.passwordSuccess.set(null), 5000);
      },
      error: (err) => {
        this.loadingPassword.set(false);
        this.passwordError.set(err.error?.error || 'Failed to change password.');
      }
    });
  }

  onUpdateCurrency(): void {
    if (this.currencyForm.invalid) {
      return;
    }

    this.loadingCurrency.set(true);
    this.currencySuccess.set(null);
    this.currencyError.set(null);

    const { base_currency } = this.currencyForm.getRawValue();

    this.finance.updatePreferences(base_currency).subscribe({
      next: () => {
        this.loadingCurrency.set(false);
        this.currencySuccess.set('Preferred currency updated.');
        // Reload summary to reflect currency changes instantly
        this.finance.loadSummary();
        setTimeout(() => this.currencySuccess.set(null), 5000);
      },
      error: (err) => {
        this.loadingCurrency.set(false);
        this.currencyError.set(err.error?.error || 'Failed to update currency preferences.');
      }
    });
  }
}
