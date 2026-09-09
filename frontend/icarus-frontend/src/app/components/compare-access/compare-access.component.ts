import { Component, signal, computed, inject, OnInit, OnDestroy, viewChild, TemplateRef } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Subscription, Subject, forkJoin, of } from 'rxjs';
import { debounceTime, distinctUntilChanged, switchMap } from 'rxjs/operators';
import { AuthService, UserSummary } from '../../services/auth.service';
import { AdminService } from '../../services/admin.service';
import { WorkflowService, AccessCart, CartItem } from '../../services/workflow.service';
import { PlatformService } from '../../services/platform.service';

// Talos UI
import { TalosCardComponent, TalosCardBodyComponent, TalosCardHeaderComponent } from '@weeehaoooz/talos-ui/layout';
import { TalosDataGridComponent, type TalosGridColDef } from '@weeehaoooz/talos-ui/data-display/data-grid';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';
import { TalosButtonGroupComponent, TalosButtonGroupItemDirective } from '@weeehaoooz/talos-ui/button/button-group';
import { TalosStatusTagComponent } from '@weeehaoooz/talos-ui/data-display/status-tag';
import { TalosFormFieldComponent } from '@weeehaoooz/talos-ui/form/form-field';
import { TalosPrefixDirective, TalosSuffixDirective } from '@weeehaoooz/talos-ui/form/affix';
import { TalosInputDirective } from '@weeehaoooz/talos-ui/form/input';
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';

// Lucide Icons
import {
  LucideSearch,
  LucideX,
  LucidePlus,
  LucideCheck,
  LucideTrash2,
  LucideUsers,
  LucideColumns2,
  LucideLayers,
  LucideSave,
  LucideSend,
  LucideShoppingCart
} from '@lucide/angular';

interface Role {
  id?: string;
  module_id: string;
  app_code?: string;
  name: string;
  description: string;
}

interface ComparisonRow {
  role: Role;
  iHave: boolean;
  targetHas: boolean;
}

@Component({
  selector: 'app-compare-access',
  imports: [
    FormsModule,
    TalosCardComponent,
    TalosCardBodyComponent,
    TalosCardHeaderComponent,
    TalosDataGridComponent,
    TalosButtonDirective,
    TalosButtonGroupComponent,
    TalosButtonGroupItemDirective,
    TalosStatusTagComponent,
    TalosFormFieldComponent,
    TalosPrefixDirective,
    TalosSuffixDirective,
    TalosInputDirective,
    TalosAlertComponent,
    LucideSearch,
    LucideX,
    LucidePlus,
    LucideCheck,
    LucideTrash2,
    LucideUsers,
    LucideColumns2,
    LucideLayers,
    LucideSave,
    LucideSend,
    LucideShoppingCart
  ],
  templateUrl: './compare-access.component.html',
  styleUrl: './compare-access.component.scss',
  host: {
    '(document:keydown.escape)': 'onEscapeKey()'
  }
})
export class CompareAccessComponent implements OnInit, OnDestroy {
  private readonly authService = inject(AuthService);
  private readonly adminService = inject(AdminService);
  private readonly workflowService = inject(WorkflowService);
  private readonly platformService = inject(PlatformService);

  // ── Template Refs for Talos Data Grid ────────────────────────────────────────
  readonly nameTmpl = viewChild<TemplateRef<any>>('nameTmpl');
  readonly moduleTmpl = viewChild<TemplateRef<any>>('moduleTmpl');
  readonly appTmpl = viewChild<TemplateRef<any>>('appTmpl');
  readonly youTmpl = viewChild<TemplateRef<any>>('youTmpl');
  readonly targetTmpl = viewChild<TemplateRef<any>>('targetTmpl');
  readonly sbsActionTmpl = viewChild<TemplateRef<any>>('sbsActionTmpl');

  readonly tabNameTmpl = viewChild<TemplateRef<any>>('tabNameTmpl');
  readonly tabModuleTmpl = viewChild<TemplateRef<any>>('tabModuleTmpl');
  readonly tabAppTmpl = viewChild<TemplateRef<any>>('tabAppTmpl');
  readonly tabActionTmpl = viewChild<TemplateRef<any>>('tabActionTmpl');

  // ── User directory & search ─────────────────────────────────────────────────
  readonly allUsers = signal<UserSummary[]>([]);
  readonly isLoadingUsers = signal(false);
  readonly userSearchQuery = signal('');
  readonly selectedUser = signal<UserSummary | null>(null);
  readonly isDropdownOpen = signal(false);

  // ── Search within comparison roles ──────────────────────────────────────────
  readonly roleSearchQuery = signal('');

  readonly filteredUsers = computed(() => {
    const q = this.userSearchQuery().toLowerCase().trim();
    const currentUsername = this.authService.currentUser()?.sub;
    const users = this.allUsers().filter(u => u.username !== currentUsername);
    if (!q) return users.slice(0, 20);
    return users.filter(u =>
      u.username.toLowerCase().includes(q) ||
      u.first_name.toLowerCase().includes(q) ||
      u.last_name.toLowerCase().includes(q)
    ).slice(0, 20);
  });

  // ── Role data ───────────────────────────────────────────────────────────────
  readonly myRoles = signal<Role[]>([]);
  readonly targetUserRoles = signal<Role[]>([]);
  readonly isLoadingTargetRoles = signal(false);
  readonly isLoadingRoles = signal(false);

  readonly modules = signal<any[]>([]);
  readonly applications = signal<any[]>([]);

  // ── Comparison computed sets ─────────────────────────────────────────────────

  private readonly myRoleNames = computed(() =>
    new Set(this.myRoles().map(r => r.name))
  );

  private readonly targetRoleNames = computed(() =>
    new Set(this.targetUserRoles().map(r => r.name))
  );

  /** Roles the target has that I'm missing — actionable (Add to Cart) */
  readonly missingRoles = computed<Role[]>(() => {
    const myNames = this.myRoleNames();
    return this.targetUserRoles().filter(r => !myNames.has(r.name));
  });

  /** Roles both users have */
  readonly sharedRoles = computed<Role[]>(() => {
    const myNames = this.myRoleNames();
    return this.targetUserRoles().filter(r => myNames.has(r.name));
  });

  /** Roles only I have (target doesn't have them) */
  readonly onlyMyRoles = computed<Role[]>(() => {
    const targetNames = this.targetRoleNames();
    return this.myRoles().filter(r => !targetNames.has(r.name));
  });

  // ── Cart state (mirrors request-access pattern) ──────────────────────────────
  readonly pendingRoles = signal<Role[]>([]);
  readonly currentCart = signal<AccessCart | null>(null);
  readonly cartId = signal<string | null>(null);

  readonly isSavingDraft = signal(false);
  readonly draftMessage = signal<string | null>(null);
  readonly draftError = signal<string | null>(null);

  readonly isSubmitting = signal(false);
  readonly submitMessage = signal<string | null>(null);
  readonly submitError = signal<string | null>(null);

  justification = '';

  readonly cartItemCount = computed(() => {
    const saved = this.currentCart()?.items?.length ?? 0;
    return saved > 0 ? saved : this.pendingRoles().length;
  });

  readonly cartRoleIds = computed(() => {
    const cart = this.currentCart();
    if (cart?.items?.length) {
      return new Set(cart.items.map(i => i.role_id));
    }
    return new Set(this.pendingRoles().map(r => r.id).filter(Boolean) as string[]);
  });

  readonly hasSavedDraft = computed(() => !!this.cartId());

  // ── UI state ─────────────────────────────────────────────────────────────────
  readonly activeSection = signal<'missing' | 'shared' | 'mine'>('missing');
  readonly viewMode = signal<'side-by-side' | 'tabs'>('side-by-side');

  /** Unified diff table — all roles from both users, sorted: missing first, then shared, then only-mine. */
  readonly comparisonRows = computed<ComparisonRow[]>(() => {
    const myNames = this.myRoleNames();
    const targetNames = this.targetRoleNames();

    // Build a map of role name → role object (prefer richer target data for shared roles)
    const roleMap = new Map<string, Role>();
    for (const r of this.myRoles()) roleMap.set(r.name, r);
    for (const r of this.targetUserRoles()) roleMap.set(r.name, r);

    const rows: ComparisonRow[] = [];
    for (const [name, role] of roleMap.entries()) {
      rows.push({
        role,
        iHave: myNames.has(name),
        targetHas: targetNames.has(name),
      });
    }

    // Sort: missing (target has, I don't) → shared → only mine
    return rows.sort((a, b) => {
      const score = (r: ComparisonRow) =>
        (!r.iHave && r.targetHas) ? 0 : (r.iHave && r.targetHas) ? 1 : 2;
      const diff = score(a) - score(b);
      return diff !== 0 ? diff : a.role.name.localeCompare(b.role.name);
    });
  });

  // ── Filtered role subsets (driven by roleSearchQuery) ────────────────────────
  readonly filteredComparisonRows = computed(() => {
    const q = this.roleSearchQuery().toLowerCase().trim();
    const rows = this.comparisonRows();
    if (!q) return rows;
    return rows.filter(r =>
      r.role.name.toLowerCase().includes(q) ||
      (r.role.description && r.role.description.toLowerCase().includes(q)) ||
      (r.role.module_id && r.role.module_id.toLowerCase().includes(q)) ||
      (r.role.app_code && r.role.app_code.toLowerCase().includes(q))
    );
  });

  readonly filteredMissingRoles = computed(() => {
    const q = this.roleSearchQuery().toLowerCase().trim();
    const list = this.missingRoles();
    if (!q) return list;
    return list.filter(r =>
      r.name.toLowerCase().includes(q) ||
      (r.description && r.description.toLowerCase().includes(q)) ||
      (r.module_id && r.module_id.toLowerCase().includes(q)) ||
      (r.app_code && r.app_code.toLowerCase().includes(q))
    );
  });

  readonly filteredSharedRoles = computed(() => {
    const q = this.roleSearchQuery().toLowerCase().trim();
    const list = this.sharedRoles();
    if (!q) return list;
    return list.filter(r =>
      r.name.toLowerCase().includes(q) ||
      (r.description && r.description.toLowerCase().includes(q)) ||
      (r.module_id && r.module_id.toLowerCase().includes(q)) ||
      (r.app_code && r.app_code.toLowerCase().includes(q))
    );
  });

  readonly filteredOnlyMyRoles = computed(() => {
    const q = this.roleSearchQuery().toLowerCase().trim();
    const list = this.onlyMyRoles();
    if (!q) return list;
    return list.filter(r =>
      r.name.toLowerCase().includes(q) ||
      (r.description && r.description.toLowerCase().includes(q)) ||
      (r.module_id && r.module_id.toLowerCase().includes(q)) ||
      (r.app_code && r.app_code.toLowerCase().includes(q))
    );
  });

  // ── Data Grid Column Configurations ──────────────────────────────────────────
  readonly sideBySideColumns = computed<TalosGridColDef<ComparisonRow>[]>(() => {
    const targetName = this.selectedUser() ? this.getDisplayName(this.selectedUser()!) : 'Target User';
    return [
      { field: 'role.name' as any, header: 'Role Name', sortType: 'string', filterMode: 'set', minWidth: '180px', cellTemplate: this.nameTmpl() },
      { field: 'role.module_id' as any, header: 'Module', sortType: 'string', filterMode: 'set', width: '150px', cellTemplate: this.moduleTmpl() },
      { field: 'role.app_code' as any, header: 'Application', sortType: 'string', filterMode: 'set', width: '150px', cellTemplate: this.appTmpl() },
      { field: 'iHave' as any, header: 'You', align: 'center', width: '100px', filterMode: 'set', cellTemplate: this.youTmpl() },
      { field: 'targetHas' as any, header: targetName, align: 'center', width: '150px', filterMode: 'set', cellTemplate: this.targetTmpl() },
      { field: 'action' as any, header: 'Action', align: 'right', width: '120px', sortable: false, filterable: false, cellTemplate: this.sbsActionTmpl() }
    ];
  });

  readonly missingColumns = computed<TalosGridColDef<Role>[]>(() => [
    { field: 'name', header: 'Role Name', sortType: 'string', filterMode: 'set', minWidth: '180px', cellTemplate: this.tabNameTmpl() },
    { field: 'module_id', header: 'Module', sortType: 'string', filterMode: 'set', width: '150px', cellTemplate: this.tabModuleTmpl() },
    { field: 'app_code', header: 'Application', sortType: 'string', filterMode: 'set', width: '150px', cellTemplate: this.tabAppTmpl() },
    { field: 'description', header: 'Description', minWidth: '220px', formatter: (v: any) => v || 'No description available.' },
    { field: 'action' as any, header: 'Action', align: 'right', width: '120px', sortable: false, filterable: false, cellTemplate: this.tabActionTmpl() }
  ]);

  readonly sharedAndMineColumns = computed<TalosGridColDef<Role>[]>(() => [
    { field: 'name', header: 'Role Name', sortType: 'string', filterMode: 'set', minWidth: '180px', cellTemplate: this.tabNameTmpl() },
    { field: 'module_id', header: 'Module', sortType: 'string', filterMode: 'set', width: '150px', cellTemplate: this.tabModuleTmpl() },
    { field: 'app_code', header: 'Application', sortType: 'string', filterMode: 'set', width: '150px', cellTemplate: this.tabAppTmpl() },
    { field: 'description', header: 'Description', minWidth: '240px', formatter: (v: any) => v || 'No description available.' }
  ]);

  rowStatus(row: ComparisonRow): 'missing' | 'shared' | 'mine' {
    if (!row.iHave && row.targetHas) return 'missing';
    if (row.iHave && row.targetHas) return 'shared';
    return 'mine';
  }

  private subs: Subscription[] = [];
  private readonly searchSubject = new Subject<string>();

  ngOnInit(): void {
    this.loadUsers();
    this.loadMyRoles();
    this.loadModulesAndApps();
  }

  ngOnDestroy(): void {
    this.subs.forEach(s => s.unsubscribe());
  }

  // ── Data loading ─────────────────────────────────────────────────────────────

  loadUsers(): void {
    this.isLoadingUsers.set(true);
    const sub = this.authService.listUsers().subscribe({
      next: (users) => {
        this.allUsers.set(users ?? []);
        this.isLoadingUsers.set(false);
      },
      error: () => this.isLoadingUsers.set(false)
    });
    this.subs.push(sub);

    const searchSub = this.searchSubject.pipe(
      debounceTime(200),
      distinctUntilChanged()
    ).subscribe(q => this.userSearchQuery.set(q));
    this.subs.push(searchSub);
  }

  loadMyRoles(): void {
    const sub = this.authService.getMyRoles().subscribe({
      next: (roles) => this.myRoles.set(roles ?? []),
      error: () => {}
    });
    this.subs.push(sub);
  }

  loadModulesAndApps(): void {
    const modSub = this.platformService.listModules().subscribe({
      next: (data) => this.modules.set(data ?? []),
      error: () => {}
    });
    const appSub = this.platformService.listApplications().subscribe({
      next: (data) => this.applications.set(data ?? []),
      error: () => {}
    });
    this.subs.push(modSub, appSub);
  }

  loadTargetUserRoles(user: UserSummary): void {
    this.isLoadingTargetRoles.set(true);
    const sub = this.authService.getUserRoles(user.id).subscribe({
      next: (roles) => {
        this.targetUserRoles.set(roles ?? []);
        this.isLoadingTargetRoles.set(false);
        this.activeSection.set('missing');
      },
      error: () => {
        this.targetUserRoles.set([]);
        this.isLoadingTargetRoles.set(false);
      }
    });
    this.subs.push(sub);
  }

  // ── User selection ───────────────────────────────────────────────────────────

  onUserSearchInput(event: Event): void {
    const val = (event.target as HTMLInputElement).value;
    this.searchSubject.next(val);
    this.isDropdownOpen.set(true);
  }

  selectUser(user: UserSummary): void {
    this.selectedUser.set(user);
    this.isDropdownOpen.set(false);
    this.userSearchQuery.set('');
    this.loadTargetUserRoles(user);
  }

  clearSelection(): void {
    this.selectedUser.set(null);
    this.targetUserRoles.set([]);
    this.userSearchQuery.set('');
  }

  closeDropdown(): void {
    this.isDropdownOpen.set(false);
  }

  getDisplayName(user: UserSummary): string {
    const full = [user.first_name, user.last_name].filter(Boolean).join(' ');
    return full || user.username;
  }

  // ── Cart operations (mirrors request-access) ─────────────────────────────────

  addToCart(role: Role): void {
    if (!role.id) return;
    const cartId = this.cartId();
    if (cartId) {
      const sub = this.workflowService.addItemToCart(cartId, role.id, role.name).subscribe({
        next: () => this.refreshCart()
      });
      this.subs.push(sub);
    } else {
      if (!this.pendingRoles().find(r => r.id === role.id)) {
        this.pendingRoles.update(roles => [...roles, role]);
      }
    }
  }

  addAllMissing(): void {
    const cartId = this.cartId();
    const toAdd = this.missingRoles().filter(r => r.id && !this.cartRoleIds().has(r.id));
    if (toAdd.length === 0) return;

    if (cartId) {
      const ops = toAdd.map(r => this.workflowService.addItemToCart(cartId, r.id!, r.name));
      const sub = forkJoin(ops).subscribe({ next: () => this.refreshCart() });
      this.subs.push(sub);
    } else {
      const existing = new Set(this.pendingRoles().map(r => r.id));
      const newRoles = toAdd.filter(r => !existing.has(r.id));
      this.pendingRoles.update(roles => [...roles, ...newRoles]);
    }
  }

  removeFromCart(item: CartItem): void {
    const cartId = this.cartId();
    if (!cartId) return;
    const sub = this.workflowService.removeItemFromCart(cartId, item.id).subscribe({
      next: () => this.refreshCart()
    });
    this.subs.push(sub);
  }

  removePendingRole(role: Role): void {
    this.pendingRoles.update(roles => roles.filter(r => r.id !== role.id));
  }

  saveDraft(): void {
    const pending = this.pendingRoles();
    if (pending.length === 0 && !this.cartId()) {
      this.draftError.set('Add at least one role before saving a draft.');
      setTimeout(() => this.draftError.set(null), 4000);
      return;
    }
    const existingCartId = this.cartId();
    if (existingCartId) {
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
        if (pending.length === 0) return of(cart);
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
        this.draftError.set(err.error?.error ?? 'Failed to save draft.');
        this.isSavingDraft.set(false);
      }
    });
    this.subs.push(sub);
  }

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
      this.doSubmitCart(existingCartId);
    } else {
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
          this.submitError.set(err.error?.error ?? 'Failed to prepare request.');
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
        this.submitError.set(err.error?.error ?? 'Submission failed.');
        this.isSubmitting.set(false);
      }
    });
    this.subs.push(sub);
  }

  private refreshCart(): void {
    const cartId = this.cartId();
    if (!cartId) return;
    const sub = this.workflowService.getCart(cartId).subscribe({
      next: (cart) => this.currentCart.set(cart)
    });
    this.subs.push(sub);
  }

  // ── Helpers ───────────────────────────────────────────────────────────────────

  getModuleName(moduleId: string): string {
    return this.modules().find(m => m.code === moduleId)?.name ?? moduleId;
  }

  getAppName(appCode: string | undefined): string {
    if (!appCode) return '';
    return this.applications().find(a => a.code === appCode)?.name ?? appCode;
  }

  isInCart(role: Role): boolean {
    if (!role.id) return false;
    return this.cartRoleIds().has(role.id);
  }

  onEscapeKey(): void {
    if (this.isDropdownOpen()) {
      this.closeDropdown();
    }
  }
}
