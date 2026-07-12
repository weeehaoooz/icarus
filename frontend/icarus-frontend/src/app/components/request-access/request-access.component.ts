import { Component, signal, computed, inject, OnInit, OnDestroy } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { DatePipe } from '@angular/common';
import { Subscription, Subject, fromEvent, forkJoin, of } from 'rxjs';
import { debounceTime, distinctUntilChanged, switchMap } from 'rxjs/operators';
import { AuthService } from '../../services/auth.service';
import { AdminService } from '../../services/admin.service';
import { WorkflowService, AccessCart, CartItem } from '../../services/workflow.service';

interface Role {
  id?: string;
  module_id: string;
  name: string;
  description: string;
}

@Component({
  selector: 'app-request-access',
  imports: [FormsModule, RouterLink, DatePipe],
  templateUrl: './request-access.component.html',
  styleUrl: './request-access.component.scss',
  host: {
    '(document:keydown.escape)': 'onEscapeKey()'
  }
})
export class RequestAccessComponent implements OnInit, OnDestroy {
  private readonly authService = inject(AuthService);
  private readonly adminService = inject(AdminService);
  private readonly workflowService = inject(WorkflowService);

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

  // ── Actions state (withdraw / bump / delete) ────────────────────────────────
  readonly actionsLoading = signal<Record<string, 'withdraw' | 'bump' | 'delete' | null>>({});
  readonly actionError = signal<string | null>(null);
  readonly actionSuccess = signal<string | null>(null);

  // ── Selected request for details view ──────────────────────────────────────
  readonly selectedRequest = signal<AccessCart | null>(null);

  // ── Role catalogue ──────────────────────────────────────────────────────────
  readonly rawAvailableRoles = signal<Role[]>([]);
  readonly userRoles = signal<Role[]>([]);

  readonly availableRoles = computed(() => {
    const userRoleIds = new Set(this.userRoles().map(ur => ur.id).filter(Boolean));
    const userRoleNames = new Set(this.userRoles().map(ur => ur.name).filter(Boolean));
    return this.rawAvailableRoles().filter(r => !userRoleIds.has(r.id) && !userRoleNames.has(r.name));
  });

  readonly roleSearch = signal('');
  readonly isLoadingRoles = signal(false);
  readonly isLoadingMore = signal(false);
  readonly hasMore = signal(true);
  readonly searchSubject = new Subject<string>();

  // Pagination state
  private offset = 0;
  private readonly limit = 12;

  // ── History ─────────────────────────────────────────────────────────────────
  readonly cartHistory = signal<AccessCart[]>([]);
  readonly isLoadingHistory = signal(false);

  // ── Justification ───────────────────────────────────────────────────────────
  justification = '';

  private pollInterval: ReturnType<typeof setInterval> | null = null;
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
    this.resetAndLoadRoles();
    this.loadHistory();
    // NOTE: No automatic cart creation here — drafts are only created when the
    // user explicitly clicks "Save Draft" or submits the request.

    // Setup search input debounce
    const searchSub = this.searchSubject.pipe(
      debounceTime(350),
      distinctUntilChanged()
    ).subscribe(q => {
      this.roleSearch.set(q);
      this.resetAndLoadRoles();
    });
    this.subs.push(searchSub);

    // Setup infinite scroll listener
    const scrollSub = fromEvent(window, 'scroll').subscribe(() => {
      this.onWindowScroll();
    });
    this.subs.push(scrollSub);
  }

  ngOnDestroy(): void {
    this.stopPolling();
    this.subs.forEach(s => s.unsubscribe());
  }

  onSearchChange(event: Event): void {
    const val = (event.target as HTMLInputElement).value;
    this.searchSubject.next(val);
  }

  loadUserRoles(): void {
    const sub = this.authService.getMyRoles().subscribe({
      next: (roles) => {
        this.userRoles.set(roles ?? []);
      },
      error: () => {},
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

  loadHistory(): void {
    this.isLoadingHistory.set(true);
    const sub = this.workflowService.listCarts().subscribe({
      next: (carts) => {
        // Include DRAFT carts so the user can manage their own saved drafts
        this.cartHistory.set(carts);
        this.isLoadingHistory.set(false);
      },
      error: () => this.isLoadingHistory.set(false),
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
        this.loadHistory();
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
        this.startPolling(cartId);
        this.loadHistory();
      },
      error: (err) => {
        this.submitError.set(err.error?.error ?? 'Submission failed. Please try again.');
        this.isSubmitting.set(false);
      },
    });
    this.subs.push(sub);
  }

  // ── Delete Draft ────────────────────────────────────────────────────────────

  deleteDraft(cartId: string): void {
    this.actionsLoading.update(l => ({ ...l, [cartId]: 'delete' }));
    this.actionError.set(null);
    this.actionSuccess.set(null);

    const sub = this.workflowService.deleteCarts(0, 'ALL', true, [cartId]).subscribe({
      next: () => {
        this.actionSuccess.set('Draft deleted.');
        this.actionsLoading.update(l => ({ ...l, [cartId]: null }));
        // If this was the currently loaded draft, reset the cart UI
        if (this.cartId() === cartId) {
          this.cartId.set(null);
          this.currentCart.set(null);
          this.pendingRoles.set([]);
        }
        this.loadHistory();
        setTimeout(() => this.actionSuccess.set(null), 4000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error || 'Failed to delete draft.');
        this.actionsLoading.update(l => ({ ...l, [cartId]: null }));
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

  // ── Resume Draft ─────────────────────────────────────────────────────────────

  resumeDraft(cart: AccessCart): void {
    window.scrollTo({ top: 0, behavior: 'smooth' });
    // Fetch the full cart so items are populated and roles show as "In Cart"
    const sub = this.workflowService.getCart(cart.id).subscribe({
      next: (fullCart) => {
        this.cartId.set(fullCart.id);
        this.currentCart.set(fullCart);
        this.pendingRoles.set([]);
        this.justification = fullCart.justification ?? '';
      },
      error: () => {
        // Fallback to the list-provided cart data
        this.cartId.set(cart.id);
        this.currentCart.set(cart);
        this.pendingRoles.set([]);
        this.justification = cart.justification ?? '';
      }
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

  private startPolling(cartId: string): void {
    this.stopPolling();
    this.pollInterval = setInterval(() => {
      this.workflowService.getCart(cartId).subscribe({
        next: (cart) => {
          const submitted = this.cartHistory().find(c => c.id === cartId);
          if (submitted) {
            this.cartHistory.update(h => h.map(c => c.id === cartId ? cart : c));
          }
          const allDone = cart.items.every(i =>
            i.status === 'APPROVED' || i.status === 'REJECTED' || i.status === 'CANCELLED'
          );
          if (allDone) this.stopPolling();
        },
      });
    }, 10000);
  }

  private stopPolling(): void {
    if (this.pollInterval !== null) {
      clearInterval(this.pollInterval);
      this.pollInterval = null;
    }
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

  withdrawRequest(cartId: string): void {
    this.actionsLoading.update(loading => ({ ...loading, [cartId]: 'withdraw' }));
    this.actionError.set(null);
    this.actionSuccess.set(null);

    const sub = this.workflowService.withdrawCart(cartId).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || 'Request withdrawn successfully.');
        this.actionsLoading.update(loading => ({ ...loading, [cartId]: null }));
        this.loadHistory();

        // Refresh selectedRequest if it is currently open
        const currentSelected = this.selectedRequest();
        if (currentSelected && currentSelected.id === cartId) {
          this.workflowService.getCart(cartId).subscribe({
            next: (updatedCart) => this.selectedRequest.set(updatedCart)
          });
        }

        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error || 'Failed to withdraw request.');
        this.actionsLoading.update(loading => ({ ...loading, [cartId]: null }));
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

  bumpRequest(cartId: string): void {
    this.actionsLoading.update(loading => ({ ...loading, [cartId]: 'bump' }));
    this.actionError.set(null);
    this.actionSuccess.set(null);

    const sub = this.workflowService.bumpCart(cartId).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || 'Request bumped successfully. Approvers notified.');
        this.actionsLoading.update(loading => ({ ...loading, [cartId]: null }));

        // Refresh selectedRequest if it is currently open
        const currentSelected = this.selectedRequest();
        if (currentSelected && currentSelected.id === cartId) {
          this.workflowService.getCart(cartId).subscribe({
            next: (updatedCart) => this.selectedRequest.set(updatedCart)
          });
        }

        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error || 'Failed to bump request.');
        this.actionsLoading.update(loading => ({ ...loading, [cartId]: null }));
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

  selectRequest(cart: AccessCart): void {
    this.workflowService.getCart(cart.id).subscribe({
      next: (fullCart) => {
        this.selectedRequest.set(fullCart);
      },
      error: () => {
        this.selectedRequest.set(cart);
      }
    });
  }

  closeDetails(): void {
    this.selectedRequest.set(null);
  }

  onEscapeKey(): void {
    if (this.selectedRequest()) {
      this.closeDetails();
    }
  }

  get isAdmin(): boolean {
    return this.authService.isAdmin();
  }
}
