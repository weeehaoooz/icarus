import { Component, inject, computed, signal, OnInit, OnDestroy } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive, Router, NavigationEnd } from '@angular/router';
import { Subscription } from 'rxjs';
import { filter } from 'rxjs/operators';
import { AuthService } from '../../services/auth.service';
import { ThemeService } from '../../services/theme.service';
import { WorkflowService } from '../../services/workflow.service';

@Component({
  selector: 'app-dashboard',
  imports: [RouterOutlet, RouterLink, RouterLinkActive],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.scss',
  host: {
    '(document:click)': 'onDocumentClick($event)'
  }
})
export class DashboardComponent implements OnInit, OnDestroy {
  private readonly authService = inject(AuthService);
  readonly themeService = inject(ThemeService);
  private readonly router = inject(Router);
  private readonly workflowService = inject(WorkflowService);

  /** Tracks which icon is visible: 'idle' | 'entering' | 'exiting' */
  readonly toggleAnimState = signal<'idle' | 'entering' | 'exiting'>('idle');

  readonly isSettingsExpanded = signal(false);
  readonly inboxCount = signal(0);
  readonly toastMessage = signal<string | null>(null);

  readonly currentSpace = signal<'user' | 'admin'>('user');
  readonly isProfileDropdownOpen = signal(false);

  private sseSub?: Subscription;
  private inboxSub?: Subscription;
  private routerSub?: Subscription;

  constructor() {
    if (this.router.url.includes('/dashboard/settings')) {
      this.isSettingsExpanded.set(true);
    }
  }

  ngOnInit(): void {
    this.refreshInboxCount();
    this.setupSSESubscription();
    this.syncSpaceWithUrl(this.router.url);
    this.routerSub = this.router.events.pipe(
      filter((event): event is NavigationEnd => event instanceof NavigationEnd)
    ).subscribe((event) => {
      this.syncSpaceWithUrl(event.urlAfterRedirects || event.url);
    });
  }

  ngOnDestroy(): void {
    this.sseSub?.unsubscribe();
    this.inboxSub?.unsubscribe();
    this.routerSub?.unsubscribe();
  }

  refreshInboxCount(): void {
    this.inboxSub?.unsubscribe();
    this.inboxSub = this.workflowService.getInbox().subscribe({
      next: (res) => this.inboxCount.set(res.total),
      error: () => {}
    });
  }

  setupSSESubscription(): void {
    this.sseSub?.unsubscribe();
    this.sseSub = this.workflowService.connectSSE().subscribe({
      next: (event) => {
        if (event.type === 'inbox.new') {
          this.refreshInboxCount();
          this.showToast('New pending access request received in your inbox.');
        } else if (event.type === 'inbox.bumped') {
          this.refreshInboxCount();
          this.showToast('Reminder: A pending access request is awaiting your approval.');
        } else if (event.type === 'cart.updated') {
          this.showToast('One of your access requests has been updated.');
        }
      },
      error: () => {}
    });
  }

  showToast(message: string): void {
    this.toastMessage.set(message);
    setTimeout(() => {
      if (this.toastMessage() === message) {
        this.toastMessage.set(null);
      }
    }, 5000);
  }

  readonly username = computed(() => {
    return this.authService.currentUser()?.sub || 'Admin';
  });

  readonly isAdmin = computed(() => {
    return this.authService.isAdmin();
  });

  readonly isModuleOwner = computed(() => {
    return this.authService.isModuleOwner();
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

  isAdminRoute(url: string): boolean {
    const adminPaths = [
      '/dashboard/overview',
      '/dashboard/users',
      '/dashboard/clients',
      '/dashboard/roles',
      '/dashboard/workflows',
      '/dashboard/workflow-builder',
      '/dashboard/request-management',
      '/dashboard/applications',
      '/dashboard/tenants',
      '/dashboard/settings'
    ];
    return adminPaths.some(path => url.includes(path));
  }

  syncSpaceWithUrl(url: string): void {
    if (url.includes('/dashboard/modules')) {
      return;
    }
    if (this.isAdmin() && this.isAdminRoute(url)) {
      this.currentSpace.set('admin');
    } else {
      this.currentSpace.set('user');
    }
  }

  setSpace(space: 'user' | 'admin'): void {
    this.currentSpace.set(space);
    if (this.router.url.includes('/dashboard/modules')) {
      return;
    }
    if (space === 'user') {
      if (this.isAdminRoute(this.router.url)) {
        this.router.navigate(['/dashboard/my-policies']);
      }
    } else {
      if (!this.isAdminRoute(this.router.url)) {
        this.router.navigate(['/dashboard/overview']);
      }
    }
  }

  toggleProfileDropdown(event: Event): void {
    event.stopPropagation();
    this.isProfileDropdownOpen.update(val => !val);
  }

  onDocumentClick(event: Event): void {
    if (this.isProfileDropdownOpen()) {
      this.isProfileDropdownOpen.set(false);
    }
  }

  logout(): void {
    this.authService.logout();
  }
}
