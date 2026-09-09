import { Component, signal, computed, inject, OnInit } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { Location } from '@angular/common';
import { WorkflowService, AuditLog } from '../../services/workflow.service';
import {
  TalosTimelineComponent,
  TalosTimelineItem,
  TalosTimelineItemStatus
} from '@weeehaoooz/talos-ui/data-display/timeline';
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';
import { TalosStatusTagComponent } from '@weeehaoooz/talos-ui/data-display/status-tag';
import {
  LucideArrowLeft,
  LucideRotateCcw,
  LucideHistory,
  LucideShieldCheck,
  LucideActivity
} from '@lucide/angular';

@Component({
  selector: 'app-request-detail',
  imports: [
    TalosTimelineComponent,
    TalosAlertComponent,
    TalosButtonDirective,
    TalosStatusTagComponent,
    LucideArrowLeft,
    LucideRotateCcw,
    LucideHistory,
    LucideShieldCheck,
    LucideActivity
  ],
  templateUrl: './request-detail.component.html',
  styleUrl: './request-detail.component.scss'
})
export class RequestDetailComponent implements OnInit {
  private readonly workflowService = inject(WorkflowService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly location = inject(Location);

  readonly events = signal<AuditLog[]>([]);
  readonly instanceId = signal('');
  readonly cartId = signal<string | null>(null);
  readonly isLoading = signal(true);
  readonly error = signal<string | null>(null);

  readonly timelineItems = computed<TalosTimelineItem[]>(() => {
    return this.events().map((event, index) => {
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

      if (event.correlation_id) {
        metadata = { ...(metadata ?? {}), correlation_id: event.correlation_id };
      }

      return {
        id: event.id || index,
        title: this.actionLabel(event.action),
        description: event.entity_type ? `${event.entity_type}${event.entity_id ? ` • #${event.entity_id}` : ''}` : undefined,
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
    this.instanceId.set(id);
    this.cartId.set(this.route.snapshot.queryParamMap.get('cartId'));
    this.loadAuditTrail(id);
  }

  loadAuditTrail(id: string): void {
    if (!id) {
      this.error.set('No instance ID provided.');
      this.isLoading.set(false);
      return;
    }

    this.isLoading.set(true);
    this.error.set(null);

    this.workflowService.getAuditTrail(id).subscribe({
      next: (res) => {
        this.events.set(res.events ?? []);
        this.isLoading.set(false);
      },
      error: () => {
        this.error.set('Failed to load audit trail.');
        this.isLoading.set(false);
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

