import { Component, inject, computed, signal, OnInit, OnDestroy, effect, viewChild } from '@angular/core';
import { Router, NavigationEnd } from '@angular/router';
import { Subscription } from 'rxjs';
import { filter } from 'rxjs/operators';
import { RouterOutlet, RouterLink } from '@angular/router';
import { AuthService } from '../../services/auth.service';
import { ThemeService } from '../../services/theme.service';
import { WorkflowService } from '../../services/workflow.service';

// Talos UI
import { MainLayoutComponent } from '@weeehaoooz/talos-ui/layout';
import {
  SideNavComponent,
  TopNavComponent,
  type SideNavGroup,
  type SideNavUserProfile
} from '@weeehaoooz/talos-ui/nav';
import { TalosSnackbarService } from '@weeehaoooz/talos-ui/feedback/snackbar';

// Lucide icons for nav items
import {
  LucideGlobe,
  LucideUser,
  LucideShield,
  LucideInbox,
  LucideUsers,
  LucideUsers2,
  LucideMonitor,
  LucideKey,
  LucideGitBranch,
  LucideClipboardList,
  LucideCode2,
  LucideLayoutGrid,
  LucideSettings,
  LucideDatabase,
} from '@lucide/angular';

@Component({
  selector: 'app-dashboard',
  imports: [RouterOutlet, RouterLink, MainLayoutComponent, TopNavComponent, SideNavComponent],
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
  private readonly snackbar = inject(TalosSnackbarService);

  readonly sideNav = viewChild(SideNavComponent);
  readonly currentUrl = signal<string>(this.router.url);
  readonly isSettingsExpanded = signal(false);
  readonly inboxCount = signal(0);
  readonly currentSpace = signal<'user' | 'admin'>('user');
  readonly isProfileDropdownOpen = signal(false);

  private sseSub?: Subscription;
  private inboxSub?: Subscription;
  private routerSub?: Subscription;

  constructor() {
    if (this.router.url.includes('/dashboard/settings')) {
      this.isSettingsExpanded.set(true);
    }

    effect(() => {
      const activeId = this.activeNavItemId();
      const nav = this.sideNav();
      if (nav && activeId) {
        nav.activeItemId.set(activeId);
      }
    });
  }

  ngOnInit(): void {
    this.currentUrl.set(this.router.url);
    this.refreshInboxCount();
    this.setupSSESubscription();
    this.syncSpaceWithUrl(this.router.url);
    this.routerSub = this.router.events.pipe(
      filter((event): event is NavigationEnd => event instanceof NavigationEnd)
    ).subscribe((event) => {
      const url = event.urlAfterRedirects || event.url;
      this.currentUrl.set(url);
      this.syncSpaceWithUrl(url);
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
        try {
          if (event.type === 'inbox.new') {
            this.refreshInboxCount();
            this.snackbar.info('New pending access request received in your inbox.');
          } else if (event.type === 'inbox.bumped') {
            this.refreshInboxCount();
            this.snackbar.warning('Reminder: A pending access request is awaiting your approval.');
          } else if (event.type === 'cart.updated') {
            this.snackbar.info('One of your access requests has been updated.');
          }
        } catch (err) {
          console.warn('[Dashboard] Error processing incoming SSE event:', err);
        }
      },
      error: (err) => {
        console.warn('[Dashboard] SSE subscription stream error:', err);
      }
    });
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

  readonly currentUserProfile = computed<SideNavUserProfile>(() => ({
    name: this.username(),
    email: this.isAdmin() ? 'Administrator' : 'Standard User'
  }));

  /** Active nav item id derived from the current URL */
  readonly activeNavItemId = computed(() => {
    const url = this.currentUrl();
    if (url.includes('/dashboard/overview')) return 'overview';
    if (url.includes('/dashboard/users')) return 'users';
    if (url.includes('/dashboard/clients')) return 'clients';
    if (url.includes('/dashboard/roles')) return 'roles';
    if (url.includes('/dashboard/workflows')) return 'workflows';
    if (url.includes('/dashboard/workflow-builder')) return 'workflows';
    if (url.includes('/dashboard/request-management')) return 'request-management';
    if (url.includes('/dashboard/applications')) return 'applications';
    if (url.includes('/dashboard/tenants')) return 'tenants';
    if (url.includes('/dashboard/settings')) return 'settings-integrations';
    if (url.includes('/dashboard/modules')) return 'modules';
    if (url.includes('/dashboard/profile-settings')) return 'profile-settings';
    if (url.includes('/dashboard/my-policies')) return 'my-policies';
    if (url.includes('/dashboard/request-access')) return 'request-access';
    if (url.includes('/dashboard/my-requests')) return 'my-requests';
    if (url.includes('/dashboard/approval-inbox')) return 'approval-inbox';
    if (url.includes('/dashboard/compare-access')) return 'compare-access';
    return '';
  });

  /** User-space nav groups */
  readonly userNavGroups = computed<SideNavGroup[]>(() => [
    {
      title: 'My Access',
      items: [
        {
          id: 'my-policies',
          label: 'Access Overview',
          icon: LucideGlobe,
          active: this.activeNavItemId() === 'my-policies',
        },
        {
          id: 'profile-settings',
          label: 'Profile & Settings',
          icon: LucideUser,
          active: this.activeNavItemId() === 'profile-settings',
        },
      ],
    },
    {
      title: 'Access Governance',
      items: [
        {
          id: 'request-access',
          label: 'Request Access',
          icon: LucideShield,
          active: this.activeNavItemId() === 'request-access',
        },
        {
          id: 'my-requests',
          label: 'My Requests',
          icon: LucideClipboardList,
          active: this.activeNavItemId() === 'my-requests',
        },
        {
          id: 'approval-inbox',
          label: 'Approval Inbox',
          icon: LucideInbox,
          badge: this.inboxCount() > 0 ? this.inboxCount() : undefined,
          active: this.activeNavItemId() === 'approval-inbox',
        },
        {
          id: 'compare-access',
          label: 'Compare Access',
          icon: LucideUsers2,
          active: this.activeNavItemId() === 'compare-access',
        },
      ],
    },
    {
      title: 'Modules',
      items: [
        {
          id: 'modules',
          label: 'Modules',
          icon: LucideCode2,
          active: this.activeNavItemId() === 'modules',
        },
      ],
    },
  ]);

  /** Admin-space nav groups */
  readonly adminNavGroups = computed<SideNavGroup[]>(() => [
    {
      title: 'IAM',
      items: [
        {
          id: 'overview',
          label: 'Overview',
          icon: LucideLayoutGrid,
          active: this.activeNavItemId() === 'overview',
        },
        {
          id: 'users',
          label: 'Users',
          icon: LucideUsers,
          active: this.activeNavItemId() === 'users',
        },
        {
          id: 'clients',
          label: 'Clients',
          icon: LucideMonitor,
          active: this.activeNavItemId() === 'clients',
        },
        {
          id: 'roles',
          label: 'Access Policies',
          icon: LucideKey,
          active: this.activeNavItemId() === 'roles',
        },
        {
          id: 'workflows',
          label: 'Workflows',
          icon: LucideGitBranch,
          active: this.activeNavItemId() === 'workflows',
        },
        {
          id: 'request-management',
          label: 'Request Management',
          icon: LucideClipboardList,
          active: this.activeNavItemId() === 'request-management',
        },
      ],
    },
    {
      title: 'Apps',
      items: [
        {
          id: 'applications',
          label: 'Applications',
          icon: LucideLayoutGrid,
          active: this.activeNavItemId() === 'applications',
        },
      ],
    },
    {
      title: 'Modules',
      items: [
        {
          id: 'modules',
          label: 'Modules',
          icon: LucideCode2,
          active: this.activeNavItemId() === 'modules',
        },
      ],
    },
    {
      title: 'Platform',
      collapsible: true,
      items: [
        {
          id: 'tenants',
          label: 'Tenants',
          icon: LucideGlobe,
          active: this.activeNavItemId() === 'tenants',
        },
        {
          id: 'settings-integrations',
          label: 'Integrations',
          icon: LucideSettings,
          active: this.activeNavItemId() === 'settings-integrations',
        },
      ],
    },
  ]);

  /** Active nav groups based on current space */
  readonly activeNavGroups = computed<SideNavGroup[]>(() => {
    if (this.isAdmin() && this.currentSpace() === 'admin') {
      return this.adminNavGroups();
    }
    return this.userNavGroups();
  });

  onNavItemClick(itemId: string): void {
    const routeMap: Record<string, string> = {
      'my-policies': '/dashboard/my-policies',
      'profile-settings': '/dashboard/profile-settings',
      'request-access': '/dashboard/request-access',
      'my-requests': '/dashboard/my-requests',
      'approval-inbox': '/dashboard/approval-inbox',
      'compare-access': '/dashboard/compare-access',
      'modules': '/dashboard/modules',
      'overview': '/dashboard/overview',
      'users': '/dashboard/users',
      'clients': '/dashboard/clients',
      'roles': '/dashboard/roles',
      'workflows': '/dashboard/workflows',
      'request-management': '/dashboard/request-management',
      'applications': '/dashboard/applications',
      'tenants': '/dashboard/tenants',
      'settings-integrations': '/dashboard/settings/integrations',
    };
    const route = routeMap[itemId];
    if (route) {
      this.router.navigate([route]);
    }
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
