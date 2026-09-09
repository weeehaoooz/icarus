import { Component, signal, computed, inject, OnInit, OnDestroy } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { RouterLink, Router } from '@angular/router';
import { Subscription } from 'rxjs';
import { WorkflowService, AccessCart } from '../../services/workflow.service';
import { TalosCardComponent, TalosCardBodyComponent } from '@weeehaoooz/talos-ui/layout';
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';
import { TalosFormFieldComponent } from '@weeehaoooz/talos-ui/form/form-field';
import { TalosPrefixDirective, TalosSuffixDirective } from '@weeehaoooz/talos-ui/form/affix';
import { TalosInputDirective } from '@weeehaoooz/talos-ui/form/input';
import { TalosStatusTagComponent } from '@weeehaoooz/talos-ui/data-display/status-tag';
import {
  LucideSearch,
  LucideX,
  LucideFileText,
  LucideClock,
  LucideAlertCircle,
  LucideInbox,
  LucideChevronDown,
  LucideTrash2,
  LucideRotateCcw,
  LucideZap,
  LucideBan,
  LucideArrowRight,
  LucideCopy,
  LucideCheck
} from '@lucide/angular';

@Component({
  selector: 'app-my-requests',
  imports: [
    FormsModule,
    DatePipe,
    RouterLink,
    TalosCardComponent,
    TalosCardBodyComponent,
    TalosAlertComponent,
    TalosButtonDirective,
    TalosFormFieldComponent,
    TalosPrefixDirective,
    TalosSuffixDirective,
    TalosInputDirective,
    TalosStatusTagComponent,
    LucideSearch,
    LucideX,
    LucideFileText,
    LucideClock,
    LucideAlertCircle,
    LucideInbox,
    LucideChevronDown,
    LucideTrash2,
    LucideRotateCcw,
    LucideZap,
    LucideBan,
    LucideArrowRight,
    LucideCopy,
    LucideCheck
  ],
  templateUrl: './my-requests.component.html',
  styleUrl: './my-requests.component.scss'
})
export class MyRequestsComponent implements OnInit, OnDestroy {
  private readonly workflowService = inject(WorkflowService);
  private readonly router = inject(Router);

  // ── Data ─────────────────────────────────────────────────────────────────────
  readonly allRequests = signal<AccessCart[]>([]);
  readonly isLoading = signal(true);
  readonly error = signal<string | null>(null);

  // ── Filters ──────────────────────────────────────────────────────────────────
  readonly searchQuery = signal('');
  readonly statusFilter = signal('ALL');

  // ── Expanded cards ───────────────────────────────────────────────────────────
  readonly expandedCartIds = signal<Set<string>>(new Set());

  // ── Actions state ─────────────────────────────────────────────────────────────
  readonly actioningCartId = signal<string | null>(null);
  readonly actionSuccess = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly copiedCartId = signal<string | null>(null);

  copyId(id: string, event?: Event): void {
    if (event) {
      event.stopPropagation();
    }
    navigator.clipboard.writeText(id).then(() => {
      this.copiedCartId.set(id);
      setTimeout(() => {
        if (this.copiedCartId() === id) {
          this.copiedCartId.set(null);
        }
      }, 2000);
    });
  }

  // ── Derived: filtered requests ────────────────────────────────────────────────
  readonly filteredRequests = computed(() => {
    let list = this.allRequests();
    const query = this.searchQuery().trim().toLowerCase();
    const status = this.statusFilter();

    if (query) {
      list = list.filter(c =>
        c.id.toLowerCase().includes(query) ||
        (c.justification && c.justification.toLowerCase().includes(query))
      );
    }

    if (status !== 'ALL') {
      list = list.filter(c => c.status === status);
    }

    return list;
  });

  // ── Summary counts for status tabs ───────────────────────────────────────────
  readonly activeCounts = computed(() => {
    const all = this.allRequests();
    return {
      drafts: all.filter(c => c.status === 'DRAFT').length,
      active: all.filter(c => c.status === 'SUBMITTED' || c.status === 'IN_PROGRESS').length,
    };
  });

  private subs: Subscription[] = [];

  ngOnInit(): void {
    this.loadMyRequests();
  }

  ngOnDestroy(): void {
    this.subs.forEach(s => s.unsubscribe());
  }

  // ── Data Loading ──────────────────────────────────────────────────────────────

  loadMyRequests(): void {
    this.isLoading.set(true);
    this.error.set(null);

    const sub = this.workflowService.listCarts(true).subscribe({
      next: (carts) => {
        this.allRequests.set(carts ?? []);
        this.isLoading.set(false);
      },
      error: (err) => {
        this.error.set('Failed to load your requests. ' + (err.error?.error || ''));
        this.isLoading.set(false);
      }
    });
    this.subs.push(sub);
  }

  // ── Card Expand / Collapse ────────────────────────────────────────────────────

  toggleExpandCart(cartId: string): void {
    this.expandedCartIds.update(set => {
      const next = new Set(set);
      if (next.has(cartId)) {
        next.delete(cartId);
      } else {
        next.add(cartId);
      }
      return next;
    });
  }

  isCartExpanded(cartId: string): boolean {
    return this.expandedCartIds().has(cartId);
  }

  // ── Request Actions ───────────────────────────────────────────────────────────

  withdrawRequest(cartId: string): void {
    this.actioningCartId.set(cartId);
    this.actionSuccess.set(null);
    this.actionError.set(null);

    const sub = this.workflowService.withdrawCart(cartId).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || 'Request withdrawn successfully.');
        this.actioningCartId.set(null);
        this.loadMyRequests();
        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error || 'Failed to withdraw request.');
        this.actioningCartId.set(null);
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

  bumpRequest(cartId: string): void {
    this.actioningCartId.set(cartId);
    this.actionSuccess.set(null);
    this.actionError.set(null);

    const sub = this.workflowService.bumpCart(cartId).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || 'Approvers notified successfully.');
        this.actioningCartId.set(null);
        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error || 'Failed to bump request.');
        this.actioningCartId.set(null);
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

  deleteRequest(cartId: string): void {
    this.actioningCartId.set(cartId);
    this.actionSuccess.set(null);
    this.actionError.set(null);

    const sub = this.workflowService.deleteUserCart(cartId).subscribe({
      next: () => {
        this.actionSuccess.set('Request deleted successfully.');
        this.actioningCartId.set(null);
        this.loadMyRequests();
        setTimeout(() => this.actionSuccess.set(null), 4000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error || 'Failed to delete request.');
        this.actioningCartId.set(null);
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

  resumeDraft(cart: AccessCart): void {
    this.router.navigate(['/dashboard/request-access'], {
      state: { resumeCartId: cart.id }
    });
  }

  // ── Status Helpers ────────────────────────────────────────────────────────────

  mapStatus(status: string): 'success' | 'warning' | 'danger' | 'info' | 'neutral' {
    switch (status) {
      case 'APPROVED':
      case 'COMPLETED':
        return 'success';
      case 'REJECTED':
      case 'CANCELLED':
        return 'danger';
      case 'IN_PROGRESS':
      case 'SUBMITTED':
        return 'warning';
      case 'DRAFT':
      case 'ARCHIVED':
        return 'info';
      default:
        return 'neutral';
    }
  }

  statusClass(status: string): string {
    switch (status) {
      case 'APPROVED': return 'status-approved';
      case 'REJECTED': return 'status-rejected';
      case 'IN_PROGRESS': return 'status-progress';
      case 'COMPLETED': return 'status-completed';
      case 'CANCELLED': return 'status-cancelled';
      case 'ARCHIVED': return 'status-archived';
      case 'DRAFT': return 'status-draft';
      default: return 'status-pending';
    }
  }
}
