import { Component, signal, inject, OnInit, OnDestroy } from '@angular/core';
import { Subscription } from 'rxjs';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { WorkflowService, InboxItem } from '../../services/workflow.service';
import { TalosCardComponent, TalosCardBodyComponent } from '@weeehaoooz/talos-ui/layout';
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';
import { TalosInputDirective } from '@weeehaoooz/talos-ui/form/input';
import { TalosFormFieldComponent } from '@weeehaoooz/talos-ui/form/form-field';
import {
  LucideCheck,
  LucideX,
  LucideInbox,
  LucideForward
} from '@lucide/angular';

@Component({
  selector: 'app-approval-inbox',
  imports: [
    FormsModule,
    DatePipe,
    TalosCardComponent,
    TalosCardBodyComponent,
    TalosAlertComponent,
    TalosButtonDirective,
    TalosInputDirective,
    TalosFormFieldComponent,
    LucideCheck,
    LucideX,
    LucideInbox,
    LucideForward
  ],
  templateUrl: './approval-inbox.component.html',
  styleUrl: './approval-inbox.component.scss'
})
export class ApprovalInboxComponent implements OnInit, OnDestroy {
  private readonly workflowService = inject(WorkflowService);

  readonly items = signal<InboxItem[]>([]);
  readonly isLoading = signal(true);
  readonly error = signal<string | null>(null);
  readonly actioningStepId = signal<string | null>(null);
  readonly actionSuccess = signal<string | null>(null);
  readonly actionError = signal<string | null>(null);
  readonly activeComments: Record<string, string> = {};
  readonly delegateTargets: Record<string, string> = {};

  private subs: Subscription[] = [];

  ngOnInit(): void {
    this.loadInbox();
  }

  ngOnDestroy(): void {
    this.subs.forEach(s => s.unsubscribe());
  }

  loadInbox(): void {
    this.isLoading.set(true);
    const sub = this.workflowService.getInbox().subscribe({
      next: (res) => {
        this.items.set(res.items);
        this.isLoading.set(false);
      },
      error: () => {
        this.error.set('Failed to load inbox. Please try again.');
        this.isLoading.set(false);
      }
    });
    this.subs.push(sub);
  }

  approve(stepId: string): void {
    this.actioningStepId.set(stepId);
    this.actionSuccess.set(null);
    this.actionError.set(null);
    const comment = this.activeComments[stepId] ?? '';
    const sub = this.workflowService.approveStep(stepId, comment).subscribe({
      next: () => {
        this.actionSuccess.set('Step approved successfully.');
        this.actioningStepId.set(null);
        this.loadInbox();
      },
      error: (err) => {
        this.actionError.set(err.error?.error ?? 'Approval failed.');
        this.actioningStepId.set(null);
      }
    });
    this.subs.push(sub);
  }

  reject(stepId: string): void {
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
        this.actionSuccess.set('Step rejected.');
        this.actioningStepId.set(null);
        this.loadInbox();
      },
      error: (err) => {
        this.actionError.set(err.error?.error ?? 'Rejection failed.');
        this.actioningStepId.set(null);
      }
    });
    this.subs.push(sub);
  }

  delegate(stepId: string): void {
    const delegateTo = this.delegateTargets[stepId]?.trim();
    if (!delegateTo) {
      this.actionError.set('Enter a user ID to delegate to.');
      return;
    }
    this.actioningStepId.set(stepId);
    this.actionSuccess.set(null);
    this.actionError.set(null);
    const sub = this.workflowService.delegateStep(stepId, delegateTo, this.activeComments[stepId] ?? '').subscribe({
      next: () => {
        this.actionSuccess.set(`Step delegated to ${delegateTo}.`);
        this.actioningStepId.set(null);
        this.loadInbox();
      },
      error: (err) => {
        this.actionError.set(err.error?.error ?? 'Delegation failed.');
        this.actioningStepId.set(null);
      }
    });
    this.subs.push(sub);
  }
}
