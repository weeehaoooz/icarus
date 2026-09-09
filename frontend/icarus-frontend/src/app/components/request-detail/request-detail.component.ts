import { Component, signal, computed, inject, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { Location, DatePipe } from '@angular/common';
import { forkJoin, of, catchError } from 'rxjs';
import { WorkflowService, AccessCart, CartItem, AuditLog, PendingStepDetail } from '../../services/workflow.service';
import {
  TalosTimelineComponent,
  TalosTimelineItem,
  TalosTimelineItemStatus
} from '@weeehaoooz/talos-ui/data-display/timeline';
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';
import { TalosStatusTagComponent } from '@weeehaoooz/talos-ui/data-display/status-tag';
import { TalosCardComponent, TalosCardBodyComponent } from '@weeehaoooz/talos-ui/layout';
import {
  LucideArrowLeft,
  LucideRotateCcw,
  LucideHistory,
  LucideActivity,
  LucideClock,
  LucideUser,
  LucideUsers,
  LucideCopy,
  LucideCheck,
  LucideZap,
  LucideBan,
  LucideFileText,
  LucideLayers,
  LucideTrash2,
  LucideInfo
} from '@lucide/angular';

@Component({
  selector: 'app-request-detail',
  imports: [
    DatePipe,
    TalosTimelineComponent,
    TalosAlertComponent,
    TalosButtonDirective,
    TalosStatusTagComponent,
    TalosCardComponent,
    TalosCardBodyComponent,
    LucideArrowLeft,
    LucideRotateCcw,
    LucideHistory,
    LucideActivity,
    LucideClock,
    LucideUser,
    LucideUsers,
    LucideCopy,
    LucideCheck,
    LucideZap,
    LucideBan,
    LucideFileText,
    LucideLayers,
    LucideTrash2,
    LucideInfo
  ],
  templateUrl: './request-detail.component.html',
  styleUrl: './request-detail.component.scss'
})
export class RequestDetailComponent implements OnInit {
  private readonly workflowService = inject(WorkflowService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly location = inject(Location);

  readonly instanceId = signal('');
  readonly cartId = signal<string | null>(null);
  readonly cart = signal<AccessCart | null>(null);
  readonly events = signal<AuditLog[]>([]);
  readonly isLoading = signal(true);
  readonly error = signal<string | null>(null);

  // Actions & feedback
  readonly isActionLoading = signal(false);
  readonly actionSuccess = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly copiedId = signal(false);

  // Timeline role filter
  readonly selectedRoleFilter = signal<string>('ALL');

  readonly availableRoleFilters = computed(() => {
    const items = this.cart()?.items ?? [];
    return items.filter(item => item.execution_id);
  });

  readonly filteredEvents = computed(() => {
    const all = this.events();
    const filter = this.selectedRoleFilter();
    if (filter === 'ALL') {
      return all;
    }
    return all.filter(e => e.entity_id === filter || e.correlation_id?.includes(filter));
  });

  readonly timelineItems = computed<TalosTimelineItem[]>(() => {
    return this.filteredEvents().map((event, index) => {
      let metadata: Record<string, unknown> | undefined;

      if (event.after_state) {
        try {
          const parsed = JSON.parse(event.after_state);
          if (typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)) {
            metadata = parsed as Record<string, unknown>;
          } else {
            metadata = { details: event.after_state };
          }
        } catch {
          metadata = { details: event.after_state };
        }
      }

      if (event.before_state && event.before_state !== '{}') {
        metadata = { ...(metadata ?? {}), before_state: event.before_state };
      }

      if (event.correlation_id) {
        metadata = { ...(metadata ?? {}), correlation_id: event.correlation_id };
      }

      return {
        id: event.id || index,
        title: this.actionLabel(event.action),
        description: event.entity_type ? `${event.entity_type}${event.entity_id ? ` • #${event.entity_id.slice(0, 8)}` : ''}` : undefined,
        timestamp: event.occurred_at ? new Date(event.occurred_at) : undefined,
        status: this.mapActionToStatus(event.action),
        actor: event.actor_user_id ? { name: event.actor_user_id } : undefined,
        metadata,
        expandable: Boolean(metadata && Object.keys(metadata).length > 0)
      };
    });
  });

  ngOnInit(): void {
    const id = this.route.snapshot.paramMap.get('instanceId') ?? '';
    const qCartId = this.route.snapshot.queryParamMap.get('cartId');
    this.instanceId.set(id);
    this.cartId.set(qCartId);
    this.loadRequestDetails(id, qCartId);
  }

  loadRequestDetails(id: string, qCartId: string | null): void {
    if (!id && !qCartId) {
      this.error.set('No request ID or instance ID provided.');
      this.isLoading.set(false);
      return;
    }

    this.isLoading.set(true);
    this.error.set(null);

    const targetCartId = qCartId || id;

    // First attempt to load cart details
    this.workflowService.getCart(targetCartId).pipe(
      catchError(() => of(null))
    ).subscribe({
      next: (cart) => {
        if (cart) {
          this.cart.set(cart);
          this.loadAllAuditTrailsForCart(cart, id);
        } else {
          // If cart wasn't found by that ID, assume it's an execution instance ID
          this.loadAuditTrailOnly(id);
        }
      },
      error: () => {
        this.loadAuditTrailOnly(id);
      }
    });
  }

  private loadAllAuditTrailsForCart(cart: AccessCart, primaryId: string): void {
    const executionIds = new Set<string>();

    if (cart.items && cart.items.length > 0) {
      cart.items.forEach(i => {
        if (i.execution_id) {
          executionIds.add(i.execution_id);
        }
      });
    }

    // Include primaryId if not already in execution IDs
    if (primaryId && primaryId !== cart.id) {
      executionIds.add(primaryId);
    }

    if (executionIds.size === 0) {
      this.events.set([]);
      this.isLoading.set(false);
      return;
    }

    const requests = Array.from(executionIds).map(execId =>
      this.workflowService.getAuditTrail(execId).pipe(
        catchError(() => of({ instance_id: execId, events: [] as AuditLog[] }))
      )
    );

    forkJoin(requests).subscribe({
      next: (results) => {
        const allLogs: AuditLog[] = [];
        const seen = new Set<string>();

        for (const res of results) {
          if (res.events) {
            for (const ev of res.events) {
              const key = ev.id || `${ev.occurred_at}_${ev.action}_${ev.entity_id}`;
              if (!seen.has(key)) {
                seen.add(key);
                allLogs.push(ev);
              }
            }
          }
        }

        // Sort chronologically ascending
        allLogs.sort((a, b) => new Date(a.occurred_at).getTime() - new Date(b.occurred_at).getTime());
        this.events.set(allLogs);
        this.isLoading.set(false);
      },
      error: () => {
        this.events.set([]);
        this.isLoading.set(false);
      }
    });
  }

  private loadAuditTrailOnly(instanceId: string): void {
    this.workflowService.getAuditTrail(instanceId).subscribe({
      next: (res) => {
        this.events.set(res.events ?? []);
        this.isLoading.set(false);
      },
      error: () => {
        this.error.set('Failed to load request audit trail.');
        this.isLoading.set(false);
      }
    });
  }

  refresh(): void {
    const id = this.instanceId();
    const qCartId = this.cartId();
    this.loadRequestDetails(id, qCartId);
  }

  copyId(id: string): void {
    navigator.clipboard?.writeText(id).then(() => {
      this.copiedId.set(true);
      setTimeout(() => this.copiedId.set(false), 2000);
    });
  }

  bumpRequest(): void {
    const c = this.cart();
    if (!c) return;

    this.isActionLoading.set(true);
    this.actionSuccess.set(null);
    this.actionError.set(null);

    this.workflowService.bumpCart(c.id).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || 'Approvers notified successfully.');
        this.isActionLoading.set(false);
        this.refresh();
        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error || 'Failed to bump request.');
        this.isActionLoading.set(false);
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
  }

  withdrawRequest(): void {
    const c = this.cart();
    if (!c) return;

    this.isActionLoading.set(true);
    this.actionSuccess.set(null);
    this.actionError.set(null);

    this.workflowService.withdrawCart(c.id).subscribe({
      next: (res) => {
        this.actionSuccess.set(res.message || 'Request withdrawn successfully.');
        this.isActionLoading.set(false);
        this.refresh();
        setTimeout(() => this.actionSuccess.set(null), 5000);
      },
      error: (err) => {
        this.actionError.set(err.error?.error || 'Failed to withdraw request.');
        this.isActionLoading.set(false);
        setTimeout(() => this.actionError.set(null), 5000);
      }
    });
  }

  resumeDraft(): void {
    const c = this.cart();
    if (!c) return;
    this.router.navigate(['/dashboard/request-access'], {
      state: { resumeCartId: c.id }
    });
  }

  deleteRequest(): void {
    const c = this.cart();
    if (!c) return;

    this.isActionLoading.set(true);
    this.workflowService.deleteUserCart(c.id).subscribe({
      next: () => {
        this.router.navigate(['/dashboard/my-requests']);
      },
      error: (err) => {
        this.actionError.set(err.error?.error || 'Failed to delete request.');
        this.isActionLoading.set(false);
      }
    });
  }

  goBack(): void {
    const returnUrl = this.route.snapshot.queryParamMap.get('returnUrl');
    if (returnUrl) {
      this.router.navigateByUrl(returnUrl);
      return;
    }
    const from = this.route.snapshot.queryParamMap.get('from');
    if (from === 'management') {
      this.router.navigate(['/dashboard/request-management']);
      return;
    }
    if (from === 'my-requests') {
      this.router.navigate(['/dashboard/my-requests']);
      return;
    }
    if (window.history.length > 1) {
      this.location.back();
    } else {
      this.router.navigate(['/dashboard/my-requests']);
    }
  }

  mapStatus(status?: string): 'success' | 'warning' | 'danger' | 'info' | 'neutral' {
    if (!status) return 'neutral';
    switch (status) {
      case 'APPROVED':
      case 'COMPLETED':
        return 'success';
      case 'REJECTED':
      case 'CANCELLED':
        return 'danger';
      case 'IN_PROGRESS':
      case 'SUBMITTED':
      case 'PENDING':
        return 'warning';
      case 'DRAFT':
      case 'ARCHIVED':
        return 'info';
      default:
        return 'neutral';
    }
  }

  actionLabel(action: string): string {
    const labels: Record<string, string> = {
      SUBMITTED: 'Request Submitted',
      AUTO_APPROVED: 'Auto-Approved',
      STARTED: 'Workflow Started',
      ASSIGNED: 'Assigned for Review',
      APPROVED: 'Step Approved',
      REJECTED: 'Request Rejected',
      DELEGATED: 'Review Delegated',
      COMPLETED: 'Fully Approved & Completed',
      WITHDRAWN: 'Request Withdrawn',
      BUMPED: 'Approver Reminder Sent',
    };
    return labels[action] ?? action;
  }

  mapActionToStatus(action: string): TalosTimelineItemStatus {
    switch (action) {
      case 'APPROVED':
      case 'COMPLETED':
      case 'AUTO_APPROVED':
        return 'completed';
      case 'REJECTED':
        return 'error';
      case 'DELEGATED':
        return 'warning';
      case 'STARTED':
        return 'in-progress';
      case 'ASSIGNED':
      case 'SUBMITTED':
        return 'primary';
      default:
        return 'neutral';
    }
  }
}
