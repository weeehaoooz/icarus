import { Component, signal, inject, OnInit } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { WorkflowService, WorkflowDefinition, WorkflowStage, WorkflowApprover } from '../../services/workflow.service';
import { AdminService } from '../../services/admin.service';

interface FormApprover {
  resolver_type: 'USER' | 'ROLE_QUEUE';
  resolver_value: string;
}

interface FormStage {
  name: string;
  execution_mode: 'SEQUENTIAL' | 'PARALLEL';
  approval_quorum: number;
  approvers: FormApprover[];
}

@Component({
  selector: 'app-workflow-builder',
  imports: [FormsModule, RouterLink],
  templateUrl: './workflow-builder.component.html',
  styleUrl: './workflow-builder.component.scss'
})
export class WorkflowBuilderComponent implements OnInit {
  private readonly workflowService = inject(WorkflowService);
  private readonly adminService = inject(AdminService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);

  readonly roleId = signal('');
  readonly workflowName = signal('');
  readonly stages = signal<FormStage[]>([]);
  
  readonly roles = signal<any[]>([]); // for ROLE_QUEUE dropdown selection
  readonly isLoading = signal(true);
  readonly isSaving = signal(false);
  readonly error = signal<string | null>(null);
  readonly success = signal<string | null>(null);

  ngOnInit(): void {
    const id = this.route.snapshot.paramMap.get('roleId') ?? '';
    this.roleId.set(id);
    this.loadRoles();
    this.loadWorkflow();
  }

  loadRoles(): void {
    this.adminService.listRoles().subscribe({
      next: (data) => this.roles.set(data ?? []),
    });
  }

  loadWorkflow(): void {
    this.isLoading.set(true);
    this.workflowService.getWorkflowDefinition(this.roleId()).subscribe({
      next: (def) => {
        if (def) {
          this.workflowName.set(def.name);
          const formStages: FormStage[] = def.stages.map(s => ({
            name: s.name,
            execution_mode: s.execution_mode as 'SEQUENTIAL' | 'PARALLEL',
            approval_quorum: s.approval_quorum,
            approvers: s.approvers.map(a => ({
              resolver_type: a.resolver_type as 'USER' | 'ROLE_QUEUE',
              resolver_value: a.resolver_value
            }))
          }));
          this.stages.set(formStages);
        } else {
          // Initialize with one default stage
          this.workflowName.set(`Workflow for ${this.roleId()}`);
          this.stages.set([this.createDefaultStage(1)]);
        }
        this.isLoading.set(false);
      },
      error: () => {
        this.error.set('Failed to load workflow configuration.');
        this.isLoading.set(false);
      }
    });
  }

  createDefaultStage(seq: number): FormStage {
    return {
      name: `Stage ${seq}`,
      execution_mode: 'SEQUENTIAL',
      approval_quorum: 1,
      approvers: [{ resolver_type: 'ROLE_QUEUE', resolver_value: '' }]
    };
  }

  addStage(): void {
    const current = this.stages();
    this.stages.set([...current, this.createDefaultStage(current.length + 1)]);
  }

  removeStage(index: number): void {
    const current = this.stages();
    if (current.length <= 1) return;
    this.stages.set(current.filter((_, i) => i !== index));
  }

  addApprover(stageIndex: number): void {
    const current = this.stages();
    const stage = current[stageIndex];
    stage.approvers.push({ resolver_type: 'ROLE_QUEUE', resolver_value: '' });
    this.stages.set([...current]);
  }

  removeApprover(stageIndex: number, approverIndex: number): void {
    const current = this.stages();
    const stage = current[stageIndex];
    if (stage.approvers.length <= 1) return;
    stage.approvers = stage.approvers.filter((_, i) => i !== approverIndex);
    this.stages.set([...current]);
  }

  saveWorkflow(): void {
    this.error.set(null);
    this.success.set(null);

    const name = this.workflowName().trim();
    if (!name) {
      this.error.set('Workflow name is required.');
      return;
    }

    const currentStages = this.stages();
    for (let sIdx = 0; sIdx < currentStages.length; sIdx++) {
      const stage = currentStages[sIdx];
      if (!stage.name.trim()) {
        this.error.set(`Stage ${sIdx + 1} name is required.`);
        return;
      }
      if (stage.approvers.length === 0) {
        this.error.set(`Stage "${stage.name}" must have at least one approver.`);
        return;
      }
      for (let aIdx = 0; aIdx < stage.approvers.length; aIdx++) {
        const ap = stage.approvers[aIdx];
        if (!ap.resolver_value.trim()) {
          this.error.set(`Stage "${stage.name}", Approver ${aIdx + 1} is missing target user/role value.`);
          return;
        }
      }
    }

    this.isSaving.set(true);

    const payload = {
      name,
      stages: currentStages.map((s, idx) => ({
        sequence_order: idx + 1,
        execution_mode: s.execution_mode,
        name: s.name,
        approval_quorum: s.approval_quorum,
        approvers: s.approvers.map(a => ({
          resolver_type: a.resolver_type,
          resolver_value: a.resolver_value
        }))
      }))
    };

    this.workflowService.upsertWorkflowDefinition(this.roleId(), payload).subscribe({
      next: () => {
        this.success.set('Workflow configuration saved successfully as a new version.');
        this.isSaving.set(false);
        setTimeout(() => this.router.navigate(['/dashboard/roles']), 1500);
      },
      error: (err) => {
        this.error.set(err.error?.error ?? 'Failed to save workflow.');
        this.isSaving.set(false);
      }
    });
  }
}
