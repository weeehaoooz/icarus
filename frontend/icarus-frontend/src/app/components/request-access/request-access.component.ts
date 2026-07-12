import { Component, signal, computed, inject, OnInit, OnDestroy } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { DatePipe } from '@angular/common';
import { Subscription, Subject, fromEvent } from 'rxjs';
import { debounceTime, distinctUntilChanged } from 'rxjs/operators';
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

  // Cart state
  readonly currentCart = signal<AccessCart | null>(null);
  readonly cartId = signal<string | null>(null);
  readonly isSubmitting = signal(false);
  readonly submitMessage = signal<string | null>(null);
  readonly submitError = signal<string | null>(null);

  // Actions state
  readonly actionsLoading = signal<Record<string, 'withdraw' | 'bump' | null>>({});
  readonly actionError = signal<string | null>(null);
  readonly actionSuccess = signal<string | null>(null);

  // Selected request for details view
  readonly selectedRequest = signal<AccessCart | null>(null);

  // Role catalogue
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

  // History
  readonly cartHistory = signal<AccessCart[]>([]);
  readonly isLoadingHistory = signal(false);

  // Justification
  justification = '';

  private pollInterval: ReturnType<typeof setInterval> | null = null;
  private subs: Subscription[] = [];

  readonly cartItemCount = computed(() => this.currentCart()?.items?.length ?? 0);

  readonly cartRoleIds = computed(() =>
    new Set(this.currentCart()?.items?.map(i => i.role_id) ?? [])
  );

  ngOnInit(): void {
    this.loadUserRoles();
    this.resetAndLoadRoles();
    this.loadHistory();
    this.initCart();

    // Setup search input debounce
    const searchSub = this.searchSubject.pipe(
      debounceTime(350),
      distinctUntilChanged()
    ).subscribe(q => {
      this.roleSearch.set(q);
      this.resetAndLoadRoles();
    });
    this.subs.push(searchSub);

    // Setup infinite scroll scroll listener
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
        this.cartHistory.set(carts.filter(c => c.status !== 'DRAFT'));
        this.isLoadingHistory.set(false);
      },
      error: () => this.isLoadingHistory.set(false),
    });
    this.subs.push(sub);
  }

  initCart(): void {
    // Create a fresh DRAFT cart on page load
    const sub = this.workflowService.createCart('').subscribe({
      next: (cart) => {
        this.cartId.set(cart.id);
        this.currentCart.set(cart);
      },
    });
    this.subs.push(sub);
  }

  addToCart(role: Role): void {
    const cartId = this.cartId();
    if (!cartId || !role.id) return;
    const sub = this.workflowService.addItemToCart(cartId, role.id, role.name).subscribe({
      next: () => this.refreshCart(),
    });
    this.subs.push(sub);
  }

  removeFromCart(item: CartItem): void {
    const cartId = this.cartId();
    if (!cartId) return;
    const sub = this.workflowService.removeItemFromCart(cartId, item.id).subscribe({
      next: () => this.refreshCart(),
    });
    this.subs.push(sub);
  }

  submitCart(): void {
    const cartId = this.cartId();
    if (!cartId) return;

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

    const sub = this.workflowService.submitCart(cartId, this.justification).subscribe({
      next: (res) => {
        this.submitMessage.set(res.message);
        this.isSubmitting.set(false);
        this.startPolling(cartId);
        this.loadHistory();
        // Create a fresh cart for next request
        this.initCart();
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
