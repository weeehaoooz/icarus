import { Component, signal, computed, inject, OnInit, OnDestroy } from '@angular/core';
import { Subscription } from 'rxjs';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';
import { WorkflowService, AccessCart, CartItem } from '../../services/workflow.service';
import { AuthService } from '../../services/auth.service';

@Component({
  selector: 'app-request-management',
  imports: [FormsModule, DatePipe, RouterLink],
  templateUrl: './request-management.component.html',
  styleUrl: './request-management.component.css'
})
export class RequestManagementComponent implements OnInit, OnDestroy {
  private readonly workflowService = inject(WorkflowService);
  private readonly authService = inject(AuthService);

  readonly allRequests = signal<AccessCart[]>([]);
  readonly isLoading = signal(true);
  readonly error = signal<string | null>(null);

  // Filters
  readonly searchQuery = signal('');
  readonly statusFilter = signal('ALL');

  // Housekeeping state
  readonly olderThanDays = signal(30);
  readonly isHousekeepingLoading = signal(false);
  readonly housekeepingSuccess = signal<string | null>(null);
  readonly housekeepingError = signal<string | null>(null);

  // Selection and Housekeeping Batch States
  readonly selectedRequestIds = signal<Set<string>>(new Set());
  readonly bulkDays = signal(30);
  readonly bulkStatus = signal('ALL');
  readonly bulkIncludeDrafts = signal(false);
  readonly isBatchActionLoading = signal(false);

  readonly selectedCount = computed(() => this.selectedRequestIds().size);

  readonly selectedRequests = computed(() => {
    const set = this.selectedRequestIds();
    return this.allRequests().filter(r => set.has(r.id));
  });

  readonly areAllSelected = computed(() => {
    const list = this.filteredRequests();
    if (list.length === 0) return false;
    const set = this.selectedRequestIds();
    return list.every(r => set.has(r.id));
  });

  readonly hasDraftsSelected = computed(() => {
    return this.selectedRequests().some(r => r.status === 'DRAFT');
  });

  // Actions state
  readonly actioningStepId = signal<string | null>(null);
  readonly actionSuccess = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly activeComments: Record<string, string> = {};

  // Expanded carts tracking
  readonly expandedCartIds = signal<Set<string>>(new Set());

  // Derived filtered requests
  readonly filteredRequests = computed(() => {
    let list = this.allRequests();
    const query = this.searchQuery().trim().toLowerCase();
    const status = this.statusFilter();

    if (query) {
      list = list.filter(c =>
        c.id.toLowerCase().includes(query) ||
        c.requester_id.toLowerCase().includes(query) ||
        (c.justification && c.justification.toLowerCase().includes(query))
      );
    }

    if (status !== 'ALL') {
      list = list.filter(c => c.status === status);
    }

    return list;
  });

  private subs: Subscription[] = [];

  ngOnInit(): void {
    this.loadAllRequests();
  }

  ngOnDestroy(): void {
    this.subs.forEach(s => s.unsubscribe());
  }

  get isAdmin(): boolean {
    return this.authService.isAdmin();
  }

  loadAllRequests(): void {
    this.isLoading.set(true);
    this.error.set(null);

    const request$ = this.isAdmin
      ? this.workflowService.listAllCarts()
      : this.workflowService.listCarts(true); // include archived requests for standard user

    const sub = request$.subscribe({
      next: (res) => {
        this.allRequests.set(res ?? []);
        this.isLoading.set(false);
      },
      error: (err) => {
        this.error.set('Failed to load access requests. ' + (err.error?.error || ''));
        this.isLoading.set(false);
      }
    });
    this.subs.push(sub);
  }

  withdrawRequest(cartId: string): void {
    this.actioningStepId.set(cartId);
    this.actionSuccess.set(null);
    this.actionError.set(null);
    const sub = this.workflowService.withdrawCart(cartId).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || 'Request withdrawn successfully.');
        this.actioningStepId.set(null);
        this.loadAllRequests();
        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error ?? 'Failed to withdraw request.');
        this.actioningStepId.set(null);
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

  bumpRequest(cartId: string): void {
    this.actioningStepId.set(cartId);
    this.actionSuccess.set(null);
    this.actionError.set(null);
    const sub = this.workflowService.bumpCart(cartId).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || 'Request bumped successfully. Approvers notified.');
        this.actioningStepId.set(null);
        this.loadAllRequests();
        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error ?? 'Failed to bump request.');
        this.actioningStepId.set(null);
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

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

  approveStep(stepId: string): void {
    this.actioningStepId.set(stepId);
    this.actionSuccess.set(null);
    this.actionError.set(null);
    const comment = this.activeComments[stepId] ?? 'Admin override approval';
    const sub = this.workflowService.approveStep(stepId, comment).subscribe({
      next: () => {
        this.actionSuccess.set('Step approved successfully by administrator override.');
        this.actioningStepId.set(null);
        this.loadAllRequests();
      },
      error: (err) => {
        this.actionError.set(err.error?.error ?? 'Override approval failed.');
        this.actioningStepId.set(null);
      }
    });
    this.subs.push(sub);
  }

  rejectStep(stepId: string): void {
    this.actioningStepId.set(stepId);
    this.actionSuccess.set(null);
    this.actionError.set(null);
    const comment = this.activeComments[stepId] ?? '';
    if (!comment.trim()) {
      this.actionError.set('A comment is required when rejecting.');
      this.actioningStepId.set(null);
      return;
    }
    const sub = this.workflowService.rejectStep(stepId, comment).subscribe({
      next: () => {
        this.actionSuccess.set('Step rejected by administrator override.');
        this.actioningStepId.set(null);
        this.loadAllRequests();
      },
      error: (err) => {
        this.actionError.set(err.error?.error ?? 'Override rejection failed.');
        this.actioningStepId.set(null);
      }
    });
    this.subs.push(sub);
  }

  runHousekeeping(): void {
    this.isHousekeepingLoading.set(true);
    this.housekeepingSuccess.set(null);
    this.housekeepingError.set(null);

    const sub = this.workflowService.archiveCarts(this.olderThanDays()).subscribe({
      next: (res) => {
        this.housekeepingSuccess.set(res.message);
        this.isHousekeepingLoading.set(false);
        this.loadAllRequests();
        setTimeout(() => this.housekeepingSuccess.set(null), 5000);
      },
      error: (err) => {
        this.housekeepingError.set(err.error?.error ?? 'Housekeeping job failed.');
        this.isHousekeepingLoading.set(false);
        setTimeout(() => this.housekeepingError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

  toggleSelectRequest(cartId: string): void {
    this.selectedRequestIds.update(set => {
      const next = new Set(set);
      if (next.has(cartId)) {
        next.delete(cartId);
      } else {
        next.add(cartId);
      }
      return next;
    });
  }

  toggleSelectAll(): void {
    const list = this.filteredRequests();
    const set = this.selectedRequestIds();
    const allSelected = list.every(r => set.has(r.id));

    this.selectedRequestIds.update(prev => {
      const next = new Set(prev);
      if (allSelected) {
        list.forEach(r => next.delete(r.id));
      } else {
        list.forEach(r => next.add(r.id));
      }
      return next;
    });
  }

  selectByDaysAndStatus(): void {
    const days = this.bulkDays();
    const status = this.bulkStatus();
    const includeDrafts = this.bulkIncludeDrafts();

    const cutoffDate = new Date();
    cutoffDate.setDate(cutoffDate.getDate() - days);

    const matched = this.allRequests().filter(r => {
      let statusMatch = false;
      if (status === 'ALL') {
        statusMatch = ['COMPLETED', 'CANCELLED', 'ARCHIVED'].includes(r.status);
        if (includeDrafts && r.status === 'DRAFT') {
          statusMatch = true;
        }
      } else {
        statusMatch = r.status === status;
      }

      if (!statusMatch) return false;

      const dateStr = r.completed_at || r.created_at;
      const rDate = new Date(dateStr);
      return rDate <= cutoffDate;
    });

    this.selectedRequestIds.update(set => {
      const next = new Set(set);
      matched.forEach(r => next.add(r.id));
      return next;
    });

    this.actionSuccess.set(`Selected ${matched.length} requests matching criteria.`);
    setTimeout(() => this.actionSuccess.set(null), 3000);
  }

  clearSelection(): void {
    this.selectedRequestIds.set(new Set());
  }

  runBatchArchive(): void {
    const ids = Array.from(this.selectedRequestIds());
    if (ids.length === 0) return;

    if (!confirm(`Are you sure you want to archive the ${ids.length} selected request(s)?`)) {
      return;
    }

    this.isBatchActionLoading.set(true);
    this.actionSuccess.set(null);
    this.actionError.set(null);

    const sub = this.workflowService.archiveCarts(0, 'ALL', ids).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || `Successfully archived ${ids.length} requests.`);
        this.selectedRequestIds.set(new Set());
        this.isBatchActionLoading.set(false);
        this.loadAllRequests();
        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error ?? 'Failed to archive selected requests.');
        this.isBatchActionLoading.set(false);
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
    this.subs.push(sub);
  }

  runBatchDelete(): void {
    const ids = Array.from(this.selectedRequestIds());
    if (ids.length === 0) return;

    const includeDrafts = this.hasDraftsSelected();
    let msg = `Are you sure you want to permanently DELETE the ${ids.length} selected request(s)? This action cannot be undone.`;
    if (includeDrafts) {
      msg += `\nWARNING: This selection includes draft requests, which will also be deleted.`;
    }

    if (!confirm(msg)) {
      return;
    }

    this.isBatchActionLoading.set(true);
    this.actionSuccess.set(null);
    this.actionError.set(null);

    const sub = this.workflowService.deleteCarts(0, 'ALL', includeDrafts, ids).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || `Successfully deleted ${ids.length} requests.`);
        this.selectedRequestIds.set(new Set());
        this.isBatchActionLoading.set(false);
        this.loadAllRequests();
        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error ?? 'Failed to delete selected requests.');
        this.isBatchActionLoading.set(false);
        setTimeout(() => this.actionError.set(null), 5000);
      }
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
      case 'ARCHIVED': return 'status-archived';
      default: return 'status-pending';
    }
  }
}
