import { Component, signal, inject, OnInit } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { DatePipe } from '@angular/common';
import { WorkflowService, AuditLog } from '../../services/workflow.service';

@Component({
  selector: 'app-request-detail',
  imports: [DatePipe],
  templateUrl: './request-detail.component.html',
  styleUrl: './request-detail.component.scss'
})
export class RequestDetailComponent implements OnInit {
  private readonly workflowService = inject(WorkflowService);
  private readonly route = inject(ActivatedRoute);

  readonly events = signal<AuditLog[]>([]);
  readonly instanceId = signal('');
  readonly isLoading = signal(true);
  readonly error = signal<string | null>(null);

  ngOnInit(): void {
    const id = this.route.snapshot.paramMap.get('instanceId') ?? '';
    this.instanceId.set(id);
    if (!id) {
      this.error.set('No instance ID provided.');
      this.isLoading.set(false);
      return;
    }
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

  actionLabel(action: string): string {
    const labels: Record<string, string> = {
      SUBMITTED: 'Request Submitted',
      AUTO_APPROVED: 'Auto-Approved',
      STARTED: 'Workflow Started',
      ASSIGNED: 'Assigned for Review',
      APPROVED: 'Approved',
      REJECTED: 'Rejected',
      DELEGATED: 'Delegated',
      COMPLETED: 'Fully Approved',
    };
    return labels[action] ?? action;
  }

  actionClass(action: string): string {
    if (['APPROVED', 'COMPLETED', 'AUTO_APPROVED'].includes(action)) return 'event-approved';
    if (['REJECTED'].includes(action)) return 'event-rejected';
    if (['DELEGATED'].includes(action)) return 'event-delegated';
    return 'event-info';
  }
}
