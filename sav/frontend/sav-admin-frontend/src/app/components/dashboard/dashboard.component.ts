import { Component, inject, computed, signal } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive, Router } from '@angular/router';
import { AuthService } from '../../services/auth.service';
import { ThemeService } from '../../services/theme.service';

@Component({
  selector: 'app-dashboard',
  imports: [RouterOutlet, RouterLink, RouterLinkActive],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.scss'
})
export class DashboardComponent {
  private readonly authService = inject(AuthService);
  readonly themeService = inject(ThemeService);
  private readonly router = inject(Router);

  /** Tracks which icon is visible: 'idle' | 'entering' | 'exiting' */
  readonly toggleAnimState = signal<'idle' | 'entering' | 'exiting'>('idle');

  readonly isSettingsExpanded = signal(false);

  constructor() {
    if (this.router.url.includes('/dashboard/settings')) {
      this.isSettingsExpanded.set(true);
    }
  }

  readonly username = computed(() => {
    return this.authService.currentUser()?.sub || 'Admin';
  });

  isSettingsActive(): boolean {
    return this.router.url.includes('/dashboard/settings');
  }

  toggleSettings(): void {
    this.isSettingsExpanded.update(val => !val);
  }

  toggleTheme(): void {
    // Play exit → switch → enter sequence
    this.toggleAnimState.set('exiting');
    setTimeout(() => {
      this.themeService.toggle();
      this.toggleAnimState.set('entering');
      setTimeout(() => this.toggleAnimState.set('idle'), 380);
    }, 220);
  }

  logout(): void {
    this.authService.logout();
  }
}
