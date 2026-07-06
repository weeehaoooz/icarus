import { Component, signal, computed, inject, OnInit, OnDestroy } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { DatePipe } from '@angular/common';
import { Subscription } from 'rxjs';
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
  styleUrl: './request-access.component.scss'
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

  // Role catalogue
  readonly availableRoles = signal<Role[]>([]);
  readonly roleSearch = signal('');
  readonly isLoadingRoles = signal(false);

  // History
  readonly cartHistory = signal<AccessCart[]>([]);
  readonly isLoadingHistory = signal(false);

  // Justification
  justification = '';

  private pollInterval: ReturnType<typeof setInterval> | null = null;
  private subs: Subscription[] = [];

  readonly filteredRoles = computed(() => {
    const q = this.roleSearch().toLowerCase();
    return this.availableRoles().filter(r =>
      r.name.toLowerCase().includes(q) || r.description?.toLowerCase().includes(q)
    );
  });

  readonly cartItemCount = computed(() => this.currentCart()?.items?.length ?? 0);

  readonly cartRoleIds = computed(() =>
    new Set(this.currentCart()?.items?.map(i => i.role_id) ?? [])
  );

  ngOnInit(): void {
    this.loadRoles();
    this.loadHistory();
    this.initCart();
  }

  ngOnDestroy(): void {
    this.stopPolling();
    this.subs.forEach(s => s.unsubscribe());
  }

  loadRoles(): void {
    this.isLoadingRoles.set(true);
    const sub = this.adminService.listRoles().subscribe({
      next: (data: any[]) => {
        this.availableRoles.set(data ?? []);
        this.isLoadingRoles.set(false);
      },
      error: () => this.isLoadingRoles.set(false),
    });
    this.subs.push(sub);
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

    const sub = this.workflowService.submitCart(cartId).subscribe({
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
      default: return 'status-pending';
    }
  }

  get isAdmin(): boolean {
    return this.authService.isAdmin();
  }
}
