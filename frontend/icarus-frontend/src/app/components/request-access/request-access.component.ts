import { Component, signal, computed, inject, OnInit, OnDestroy } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Subscription, Subject, fromEvent, forkJoin, of } from 'rxjs';
import { debounceTime, distinctUntilChanged, switchMap } from 'rxjs/operators';
import { AuthService } from '../../services/auth.service';
import { AdminService } from '../../services/admin.service';
import { WorkflowService, AccessCart, CartItem } from '../../services/workflow.service';
import { PlatformService } from '../../services/platform.service';

// Talos UI
import { TalosCardComponent, TalosCardBodyComponent, TalosCardHeaderComponent } from '@weeehaoooz/talos-ui/layout';
import { TalosStatusTagComponent } from '@weeehaoooz/talos-ui/data-display/status-tag';
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';
import { TalosFormFieldComponent } from '@weeehaoooz/talos-ui/form/form-field';
import { TalosPrefixDirective, TalosSuffixDirective } from '@weeehaoooz/talos-ui/form/affix';
import { TalosInputDirective } from '@weeehaoooz/talos-ui/form/input';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';

// Lucide Icons
import {
  LucideSearch,
  LucideX,
  LucideCheck,
  LucideLayers,
  LucidePlus,
  LucideShoppingCart,
  LucideChevronDown,
  LucideSend,
  LucideInbox,
  LucideTrash2,
  LucideSave
} from '@lucide/angular';

interface Role {
  id?: string;
  module_id: string;
  app_code?: string;
  name: string;
  description: string;
}

interface TableRow {
  type: 'group' | 'leaf';
  key: string;
  name: string;
  count?: number;
  level: number;
  role?: Role;
  parentCollapsed: boolean;
}

@Component({
  selector: 'app-request-access',
  imports: [
    FormsModule,
    TalosCardComponent,
    TalosCardBodyComponent,
    TalosCardHeaderComponent,
    TalosStatusTagComponent,
    TalosAlertComponent,
    TalosFormFieldComponent,
    TalosPrefixDirective,
    TalosSuffixDirective,
    TalosInputDirective,
    TalosButtonDirective,
    LucideSearch,
    LucideX,
    LucideCheck,
    LucideLayers,
    LucidePlus,
    LucideShoppingCart,
    LucideChevronDown,
    LucideSend,
    LucideInbox,
    LucideTrash2,
    LucideSave
  ],
  templateUrl: './request-access.component.html',
  styleUrl: './request-access.component.scss'
})
export class RequestAccessComponent implements OnInit, OnDestroy {
  private readonly authService = inject(AuthService);
  private readonly adminService = inject(AdminService);
  private readonly workflowService = inject(WorkflowService);
  private readonly platformService = inject(PlatformService);

  // ── Local (unsaved) cart state ──────────────────────────────────────────────
  /** Roles the user has added locally, before a draft is saved to the backend */
  readonly pendingRoles = signal<Role[]>([]);

  // ── Persisted cart state (after Save Draft or during Submit) ───────────────
  readonly currentCart = signal<AccessCart | null>(null);
  readonly cartId = signal<string | null>(null);

  // ── Draft save state ────────────────────────────────────────────────────────
  readonly isSavingDraft = signal(false);
  readonly draftMessage = signal<string | null>(null);
  readonly draftError = signal<string | null>(null);

  // ── Submit state ────────────────────────────────────────────────────────────
  readonly isSubmitting = signal(false);
  readonly submitMessage = signal<string | null>(null);
  readonly submitError = signal<string | null>(null);

  // ── Role catalogue ──────────────────────────────────────────────────────────
  readonly rawAvailableRoles = signal<Role[]>([]);
  readonly userRoles = signal<Role[]>([]);

  readonly availableRoles = computed(() => {
    const userRoleIds = new Set(this.userRoles().map(ur => ur.id).filter(Boolean));
    const userRoleNames = new Set(this.userRoles().map(ur => ur.name).filter(Boolean));
    return this.rawAvailableRoles().filter(r => !userRoleIds.has(r.id) && !userRoleNames.has(r.name));
  });

  // Grouping & Filtering Signals
  readonly modules = signal<any[]>([]);
  readonly applications = signal<any[]>([]);
  readonly selectedModuleFilter = signal<string>('all');
  readonly selectedAppFilter = signal<string>('all');

  readonly isGroupingEnabled = signal(false);
  readonly groupByMode = signal<'module' | 'app'>('module');

  readonly groupBy = computed<'none' | 'module' | 'app'>(() => {
    return this.isGroupingEnabled() ? this.groupByMode() : 'none';
  });

  readonly collapsedGroups = signal<Set<string>>(new Set());

  readonly roleSearch = signal('');
  readonly isLoadingRoles = signal(false);
  readonly isLoadingMore = signal(false);
  readonly hasMore = signal(true);
  readonly searchSubject = new Subject<string>();

  // Filtered Roles (Search + Filters)
  readonly filteredRoles = computed(() => {
    let list = this.availableRoles();

    // Filter by search
    const search = this.roleSearch().toLowerCase().trim();
    if (search) {
      list = list.filter(r =>
        r.name.toLowerCase().includes(search) ||
        (r.description && r.description.toLowerCase().includes(search))
      );
    }

    // Filter by module
    const modFilter = this.selectedModuleFilter();
    if (modFilter !== 'all') {
      list = list.filter(r => r.module_id === modFilter);
    }

    // Filter by application
    const appFilter = this.selectedAppFilter();
    if (appFilter !== 'all') {
      list = list.filter(r => r.app_code === appFilter);
    }

    return list;
  });

  // Grouped Roles for UI Rendering
  readonly groupedRoles = computed(() => {
    const list = this.filteredRoles();
    const mode = this.groupBy();

    if (mode === 'none') {
      return [{ name: 'All Roles', key: 'all', roles: list }];
    }

    if (mode === 'module') {
      const groups: { name: string; key: string; roles: Role[] }[] = [];
      const modulesMap = new Map(this.modules().map(m => [m.code, m.name]));

      const tempMap = new Map<string, Role[]>();
      for (const r of list) {
        const key = r.module_id || 'unknown';
        if (!tempMap.has(key)) tempMap.set(key, []);
        tempMap.get(key)!.push(r);
      }

      for (const [key, roles] of tempMap.entries()) {
        const name = modulesMap.get(key) || key;
        groups.push({ name, key, roles });
      }

      return groups.sort((a, b) => a.name.localeCompare(b.name));
    }

    if (mode === 'app') {
      const groups: { name: string; key: string; roles: Role[] }[] = [];
      const appsMap = new Map(this.applications().map(a => [a.code, a.name]));

      const tempMap = new Map<string, Role[]>();
      for (const r of list) {
        const key = r.app_code || 'global';
        if (!tempMap.has(key)) tempMap.set(key, []);
        tempMap.get(key)!.push(r);
      }

      for (const [key, roles] of tempMap.entries()) {
        const name = key === 'global' ? 'Global / No Application' : (appsMap.get(key) || key);
        groups.push({ name, key, roles });
      }

      return groups.sort((a, b) => a.name.localeCompare(b.name));
    }

    return [];
  });

  // Flat list of rows generated from the grouping structure (Application -> Module -> Roles, or Module -> Application -> Roles)
  readonly tableRows = computed<TableRow[]>(() => {
    const mode = this.groupBy();
    const list = this.filteredRoles();
    const collapsed = this.collapsedGroups();
    const modulesMap = new Map(this.modules().map(m => [m.code, m.name]));
    const appsMap = new Map(this.applications().map(a => [a.code, a.name]));

    if (mode === 'none') {
      return list.map(r => ({
        type: 'leaf',
        key: r.id ?? '',
        name: r.name,
        level: 0,
        role: r,
        parentCollapsed: false
      }));
    }

    const rows: TableRow[] = [];

    if (mode === 'module') {
      // 1. Group roles by module_id
      const moduleGroups = new Map<string, Role[]>();
      for (const r of list) {
        const mId = r.module_id || 'unknown';
        if (!moduleGroups.has(mId)) moduleGroups.set(mId, []);
        moduleGroups.get(mId)!.push(r);
      }

      // Sort modules by name
      const sortedModuleKeys = Array.from(moduleGroups.keys()).sort((a, b) => {
        const nameA = modulesMap.get(a) || a;
        const nameB = modulesMap.get(b) || b;
        return nameA.localeCompare(nameB);
      });

      for (const mId of sortedModuleKeys) {
        const mRoles = moduleGroups.get(mId)!;
        const mName = modulesMap.get(mId) || mId;
        const mKey = `module:${mId}`;
        const mCollapsed = collapsed.has(mKey);

        // Add Module Group Row (Level 0)
        rows.push({
          type: 'group',
          key: mKey,
          name: mName,
          count: mRoles.length,
          level: 0,
          parentCollapsed: false
        });

        // 2. Group roles by app_code inside this module
        const appGroups = new Map<string, Role[]>();
        for (const r of mRoles) {
          const aCode = r.app_code || 'global';
          if (!appGroups.has(aCode)) appGroups.set(aCode, []);
          appGroups.get(aCode)!.push(r);
        }

        // Sort apps by name
        const sortedAppKeys = Array.from(appGroups.keys()).sort((a, b) => {
          const nameA = a === 'global' ? 'Global / No Application' : (appsMap.get(a) || a);
          const nameB = b === 'global' ? 'Global / No Application' : (appsMap.get(b) || b);
          return nameA.localeCompare(nameB);
        });

        for (const aCode of sortedAppKeys) {
          const aRoles = appGroups.get(aCode)!;
          const aName = aCode === 'global' ? 'Global / No Application' : (appsMap.get(aCode) || aCode);
          const aKey = `${mKey}/app:${aCode}`;
          const aCollapsed = collapsed.has(aKey);

          // Add App Group Row (Level 1)
          rows.push({
            type: 'group',
            key: aKey,
            name: aName,
            count: aRoles.length,
            level: 1,
            parentCollapsed: mCollapsed
          });

          // 3. Add Leaf Roles (Level 2)
          for (const r of aRoles) {
            rows.push({
              type: 'leaf',
              key: r.id ?? '',
              name: r.name,
              level: 2,
              role: r,
              parentCollapsed: mCollapsed || aCollapsed
            });
          }
        }
      }
    } else if (mode === 'app') {
      // 1. Group roles by app_code
      const appGroups = new Map<string, Role[]>();
      for (const r of list) {
        const aCode = r.app_code || 'global';
        if (!appGroups.has(aCode)) appGroups.set(aCode, []);
        appGroups.get(aCode)!.push(r);
      }

      // Sort apps by name
      const sortedAppKeys = Array.from(appGroups.keys()).sort((a, b) => {
        const nameA = a === 'global' ? 'Global / No Application' : (appsMap.get(a) || a);
        const nameB = b === 'global' ? 'Global / No Application' : (appsMap.get(b) || b);
        return nameA.localeCompare(nameB);
      });

      for (const aCode of sortedAppKeys) {
        const aRoles = appGroups.get(aCode)!;
        const aName = aCode === 'global' ? 'Global / No Application' : (appsMap.get(aCode) || aCode);
        const aKey = `app:${aCode}`;
        const aCollapsed = collapsed.has(aKey);

        // Add App Group Row (Level 0)
        rows.push({
          type: 'group',
          key: aKey,
          name: aName,
          count: aRoles.length,
          level: 0,
          parentCollapsed: false
        });

        // 2. Group roles by module_id inside this app
        const moduleGroups = new Map<string, Role[]>();
        for (const r of aRoles) {
          const mId = r.module_id || 'unknown';
          if (!moduleGroups.has(mId)) moduleGroups.set(mId, []);
          moduleGroups.get(mId)!.push(r);
        }

        // Sort modules by name
        const sortedModuleKeys = Array.from(moduleGroups.keys()).sort((a, b) => {
          const nameA = modulesMap.get(a) || a;
          const nameB = modulesMap.get(b) || b;
          return nameA.localeCompare(nameB);
        });

        for (const mId of sortedModuleKeys) {
          const mRoles = moduleGroups.get(mId)!;
          const mName = modulesMap.get(mId) || mId;
          const mKey = `${aKey}/module:${mId}`;
          const mCollapsed = collapsed.has(mKey);

          // Add Module Group Row (Level 1)
          rows.push({
            type: 'group',
            key: mKey,
            name: mName,
            count: mRoles.length,
            level: 1,
            parentCollapsed: aCollapsed
          });

          // 3. Add Leaf Roles (Level 2)
          for (const r of mRoles) {
            rows.push({
              type: 'leaf',
              key: r.id ?? '',
              name: r.name,
              level: 2,
              role: r,
              parentCollapsed: aCollapsed || mCollapsed
            });
          }
        }
      }
    }

    return rows;
  });

  // Only rows that are not collapsed are visible in the grid
  readonly visibleRows = computed<TableRow[]>(() => {
    return this.tableRows().filter(r => !r.parentCollapsed);
  });

  toggleGrouping(): void {
    this.isGroupingEnabled.update(enabled => !enabled);
    this.collapsedGroups.set(new Set());
  }

  toggleGroup(key: string): void {
    this.collapsedGroups.update(set => {
      const newSet = new Set(set);
      if (newSet.has(key)) {
        newSet.delete(key);
      } else {
        newSet.add(key);
      }
      return newSet;
    });
  }

  expandAll(): void {
    this.collapsedGroups.set(new Set());
  }

  collapseAll(): void {
    const keys: string[] = [];
    for (const row of this.tableRows()) {
      if (row.type === 'group') {
        keys.push(row.key);
      }
    }
    this.collapsedGroups.set(new Set(keys));
  }

  isGroupCollapsed(key: string): boolean {
    return this.collapsedGroups().has(key);
  }

  // Pagination state (increased limit to fetch all for client-side grouping)
  private offset = 0;
  private readonly limit = 1000;

  // ── Justification ───────────────────────────────────────────────────────────
  justification = '';

  private subs: Subscription[] = [];

  // ── Computed helpers ─────────────────────────────────────────────────────────

  /** Total items in the active cart: persisted items OR local pending roles */
  readonly cartItemCount = computed(() => {
    const saved = this.currentCart()?.items?.length ?? 0;
    return saved > 0 ? saved : this.pendingRoles().length;
  });

  /** Role IDs currently in the active cart (persisted or pending) */
  readonly cartRoleIds = computed(() => {
    const cart = this.currentCart();
    if (cart?.items?.length) {
      return new Set(cart.items.map(i => i.role_id));
    }
    return new Set(this.pendingRoles().map(r => r.id).filter(Boolean) as string[]);
  });

  /** True when we have a saved (persisted) DRAFT cart */
  readonly hasSavedDraft = computed(() => !!this.cartId());

  ngOnInit(): void {
    this.loadUserRoles();
    this.loadModulesAndApps();
    this.resetAndLoadRoles();

    // Check if we are resuming a draft from history state
    const state = history.state;
    if (state && state.resumeCartId) {
      this.loadDraftToResume(state.resumeCartId);
    }

    // Setup search input debounce
    const searchSub = this.searchSubject.pipe(
      debounceTime(250),
      distinctUntilChanged()
    ).subscribe(q => {
      this.roleSearch.set(q);
    });
    this.subs.push(searchSub);
  }

  ngOnDestroy(): void {
    this.subs.forEach(s => s.unsubscribe());
  }

  onSearchChange(event: Event): void {
    const val = (event.target as HTMLInputElement).value;
    this.searchSubject.next(val);
  }

  loadModulesAndApps(): void {
    const modSub = this.platformService.listModules().subscribe({
      next: (data) => this.modules.set(data ?? []),
      error: () => { }
    });
    const appSub = this.platformService.listApplications().subscribe({
      next: (data) => this.applications.set(data ?? []),
      error: () => { }
    });
    this.subs.push(modSub, appSub);
  }

  loadUserRoles(): void {
    const sub = this.authService.getMyRoles().subscribe({
      next: (roles) => {
        this.userRoles.set(roles ?? []);
      },
      error: () => { },
    });
    this.subs.push(sub);
  }

  resetAndLoadRoles(): void {
    this.offset = 0;
    this.hasMore.set(true);
    this.rawAvailableRoles.set([]);
    this.loadRoles(true);
  }

  loadRoles(isInitial: boolean = false): void {
    if (isInitial) {
      this.isLoadingRoles.set(true);
    } else {
      this.isLoadingMore.set(true);
    }

    const sub = this.adminService.listRoles(this.roleSearch(), this.limit, this.offset).subscribe({
      next: (data: any[]) => {
        const roles = data ?? [];
        if (isInitial) {
          this.rawAvailableRoles.set(roles);
          this.isLoadingRoles.set(false);
        } else {
          this.rawAvailableRoles.update(existing => [...existing, ...roles]);
          this.isLoadingMore.set(false);
        }

        if (roles.length < this.limit) {
          this.hasMore.set(false);
        }
      },
      error: () => {
        if (isInitial) {
          this.isLoadingRoles.set(false);
        } else {
          this.isLoadingMore.set(false);
        }
      },
    });
    this.subs.push(sub);
  }

  loadMoreRoles(): void {
    if (this.isLoadingRoles() || this.isLoadingMore() || !this.hasMore()) return;
    this.offset += this.limit;
    this.loadRoles(false);
  }

  onWindowScroll(): void {
    if (this.isLoadingRoles() || this.isLoadingMore() || !this.hasMore()) return;
    const pos = window.innerHeight + window.scrollY;
    const max = document.documentElement.scrollHeight;
    if (pos >= max - 200) {
      this.loadMoreRoles();
    }
  }

  loadDraftToResume(cartId: string): void {
    const sub = this.workflowService.getCart(cartId).subscribe({
      next: (fullCart) => {
        this.cartId.set(fullCart.id);
        this.currentCart.set(fullCart);
        this.pendingRoles.set([]);
        this.justification = fullCart.justification ?? '';
      },
      error: () => { }
    });
    this.subs.push(sub);
  }

  // ── Add / Remove (local-only until draft is saved) ─────────────────────────

  addToCart(role: Role): void {
    if (!role.id) return;
    const cartId = this.cartId();

    if (cartId) {
      // Cart already persisted — add directly via API
      const sub = this.workflowService.addItemToCart(cartId, role.id, role.name).subscribe({
        next: () => this.refreshCart(),
      });
      this.subs.push(sub);
    } else {
      // No saved cart yet — track locally
      this.pendingRoles.update(roles => [...roles, role]);
    }
  }

  removeFromCart(item: CartItem): void {
    const cartId = this.cartId();
    if (!cartId) return;
    const sub = this.workflowService.removeItemFromCart(cartId, item.id).subscribe({
      next: () => this.refreshCart(),
    });
    this.subs.push(sub);
  }

  removePendingRole(role: Role): void {
    this.pendingRoles.update(roles => roles.filter(r => r.id !== role.id));
  }

  // ── Save Draft ──────────────────────────────────────────────────────────────

  /**
   * Persists the current local selection to the backend as a DRAFT.
   * Only called when the user explicitly clicks "Save Draft".
   */
  saveDraft(): void {
    const pending = this.pendingRoles();
    if (pending.length === 0 && !this.cartId()) {
      this.draftError.set('Add at least one role before saving a draft.');
      setTimeout(() => this.draftError.set(null), 4000);
      return;
    }

    const existingCartId = this.cartId();
    if (existingCartId) {
      // Already persisted — nothing more to do
      this.draftMessage.set('Draft is already saved.');
      setTimeout(() => this.draftMessage.set(null), 3000);
      return;
    }

    this.isSavingDraft.set(true);
    this.draftMessage.set(null);
    this.draftError.set(null);

    const sub = this.workflowService.createCart(this.justification).pipe(
      switchMap(cart => {
        this.cartId.set(cart.id);
        this.currentCart.set(cart);
        if (pending.length === 0) {
          return of(cart);
        }
        const addOps = pending.map(role =>
          this.workflowService.addItemToCart(cart.id, role.id!, role.name)
        );
        return forkJoin(addOps).pipe(
          switchMap(() => this.workflowService.getCart(cart.id))
        );
      })
    ).subscribe({
      next: (cart) => {
        this.currentCart.set(cart as AccessCart);
        this.pendingRoles.set([]);
        this.isSavingDraft.set(false);
        this.draftMessage.set('Draft saved successfully.');
        setTimeout(() => this.draftMessage.set(null), 4000);
      },
      error: (err) => {
        this.draftError.set(err.error?.error ?? 'Failed to save draft. Please try again.');
        this.isSavingDraft.set(false);
      },
    });
    this.subs.push(sub);
  }

  // ── Submit ──────────────────────────────────────────────────────────────────

  submitCart(): void {
    if (this.cartItemCount() === 0) {
      this.submitError.set('Add at least one role before submitting.');
      return;
    }
    if (this.justification.trim().length < 10) {
      this.submitError.set('Justification must be at least 10 characters.');
      return;
    }

    this.isSubmitting.set(true);
    this.submitMessage.set(null);
    this.submitError.set(null);

    const existingCartId = this.cartId();
    if (existingCartId) {
      // Already have a persisted draft — submit directly
      this.doSubmitCart(existingCartId);
    } else {
      // No saved draft — create cart, add items, then submit
      const pending = this.pendingRoles();
      const sub = this.workflowService.createCart('').pipe(
        switchMap(cart => {
          this.cartId.set(cart.id);
          const addOps = pending.map(role =>
            this.workflowService.addItemToCart(cart.id, role.id!, role.name)
          );
          return forkJoin(addOps.length ? addOps : [of(null)]).pipe(
            switchMap(() => of(cart.id))
          );
        })
      ).subscribe({
        next: (id) => {
          this.pendingRoles.set([]);
          this.doSubmitCart(id as string);
        },
        error: (err) => {
          this.submitError.set(err.error?.error ?? 'Failed to prepare request. Please try again.');
          this.isSubmitting.set(false);
        }
      });
      this.subs.push(sub);
    }
  }

  private doSubmitCart(cartId: string): void {
    const sub = this.workflowService.submitCart(cartId, this.justification).subscribe({
      next: (res) => {
        this.submitMessage.set(res.message);
        this.isSubmitting.set(false);
        this.cartId.set(null);
        this.currentCart.set(null);
        this.pendingRoles.set([]);
        this.justification = '';
      },
      error: (err) => {
        this.submitError.set(err.error?.error ?? 'Submission failed. Please try again.');
        this.isSubmitting.set(false);
      },
    });
    this.subs.push(sub);
  }

  private refreshCart(): void {
    const cartId = this.cartId();
    if (!cartId) return;
    const sub = this.workflowService.getCart(cartId).subscribe({
      next: (cart) => this.currentCart.set(cart),
    });
    this.subs.push(sub);
  }

  statusClass(status: string): string {
    switch (status) {
      case 'APPROVED': return 'status-approved';
      case 'REJECTED': return 'status-rejected';
      case 'IN_PROGRESS': return 'status-progress';
      case 'COMPLETED': return 'status-completed';
      case 'CANCELLED': return 'status-cancelled';
      case 'DRAFT': return 'status-draft';
      default: return 'status-pending';
    }
  }

  get isAdmin(): boolean {
    return this.authService.isAdmin();
  }
}
