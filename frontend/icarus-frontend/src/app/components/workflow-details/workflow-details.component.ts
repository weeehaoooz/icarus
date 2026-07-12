import { Component, signal, inject, OnInit, computed } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { WorkflowService, Workflow } from '../../services/workflow.service';
import { AdminService } from '../../services/admin.service';
import { DagPreviewComponent, FormNode } from '../workflow-builder/dag-preview/dag-preview.component';

@Component({
  selector: 'app-workflow-details',
  imports: [FormsModule, DagPreviewComponent],
  templateUrl: './workflow-details.component.html',
  styleUrl: './workflow-details.component.scss',
})
export class WorkflowDetailsComponent implements OnInit {
  private readonly workflowService = inject(WorkflowService);
  private readonly adminService = inject(AdminService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);

  readonly workflow = signal<Workflow | null>(null);
  readonly roles = signal<any[]>([]);
  readonly isLoading = signal(true);
  readonly error = signal<string | null>(null);
  readonly success = signal<string | null>(null);

  // Mapped role IDs for this workflow
  readonly mappedRoleIds = signal<string[]>([]);

  // Map role modal
  readonly showMapModal = signal(false);
  readonly selectedRoleId = signal('');
  readonly isMapping = signal(false);

  // Unmap confirm dialog
  readonly showUnmapConfirm = signal(false);
  readonly unmappingRoleId = signal('');
  readonly isUnmapping = signal(false);

  /** DAG nodes derived from workflow.nodes, converted to FormNode shape for DagPreviewComponent. */
  readonly dagNodes = computed<FormNode[]>(() => {
    const wf = this.workflow();
    if (!wf?.nodes) return [];
    return wf.nodes.map(n => {
      const config = (n.config ?? {}) as {
        policy?: 'ANY' | 'ALL' | 'QUORUM';
        min_approvals?: number;
        candidates?: { type?: 'USER' | 'ROLE'; value?: string }[];
      };
      return {
        id: n.id,
        name: n.name,
        type: n.type ?? 'APPROVAL',
        depends_on: n.depends_on ?? [],
        candidates: (config.candidates ?? []).map(c => ({ type: c.type ?? 'ROLE', value: c.value ?? '' })),
        policy: config.policy ?? 'ANY',
        min_approvals: config.min_approvals ?? 1,
        max_retries: n.max_retries ?? 0,
        retry_interval: n.retry_interval_seconds ?? n.retry_interval ?? 0,
      } satisfies FormNode;
    });
  });

  /** Roles that are NOT yet mapped to this workflow and can be added. */
  readonly availableRoles = computed(() => {
    const mapped = new Set(this.mappedRoleIds());
    return this.roles().filter(r => !mapped.has(r.id || r.name));
  });

  ngOnInit(): void {
    const id = this.route.snapshot.paramMap.get('id') ?? '';
    this.loadWorkflow(id);
    this.loadRoles();
  }

  private loadWorkflow(id: string): void {
    this.isLoading.set(true);
    this.workflowService.getWorkflowDefinitionById(id).subscribe({
      next: wf => {
        this.workflow.set(wf);
        // Fetch role mappings by loading all templates and finding this one
        this.workflowService.listWorkflowTemplates().subscribe({
          next: templates => {
            const found = templates.find(t => t.id === id);
            this.mappedRoleIds.set(found?.mapped_role_ids ?? []);
            this.isLoading.set(false);
          },
          error: () => {
            this.isLoading.set(false);
          },
        });
      },
      error: () => {
        this.error.set('Failed to load workflow details.');
        this.isLoading.set(false);
      },
    });
  }

  private loadRoles(): void {
    this.adminService.listRoles().subscribe({
      next: roles => this.roles.set(roles ?? []),
    });
  }

  statusClass(status: string): string {
    if (status === 'ACTIVE') return 'status-active';
    if (status === 'SUPERSEDED') return 'status-superseded';
    return 'status-draft';
  }

  // ── Map modal ──────────────────────────────────────────────────────────────

  openMapModal(): void {
    this.selectedRoleId.set('');
    this.error.set(null);
    this.showMapModal.set(true);
  }

  closeMapModal(): void {
    this.showMapModal.set(false);
  }

  confirmMap(): void {
    const roleId = this.selectedRoleId();
    const workflowId = this.workflow()?.id;
    if (!roleId || !workflowId) return;

    this.isMapping.set(true);
    this.workflowService.mapWorkflowToRole(roleId, workflowId).subscribe({
      next: () => {
        this.isMapping.set(false);
        this.closeMapModal();
        this.mappedRoleIds.update(ids => [...ids, roleId]);
        this.success.set(`Role "${roleId}" assigned to this workflow.`);
        setTimeout(() => this.success.set(null), 4000);
      },
      error: err => {
        this.error.set(err.error?.error ?? 'Failed to map workflow to role.');
        this.isMapping.set(false);
      },
    });
  }

  // ── Unmap confirm dialog ───────────────────────────────────────────────────

  promptUnmap(roleId: string): void {
    this.unmappingRoleId.set(roleId);
    this.showUnmapConfirm.set(true);
  }

  cancelUnmap(): void {
    this.showUnmapConfirm.set(false);
    this.unmappingRoleId.set('');
  }

  confirmUnmap(): void {
    const roleId = this.unmappingRoleId();
    if (!roleId) return;
    this.isUnmapping.set(true);
    this.workflowService.unmapWorkflowFromRole(roleId).subscribe({
      next: () => {
        this.isUnmapping.set(false);
        this.showUnmapConfirm.set(false);
        this.mappedRoleIds.update(ids => ids.filter(id => id !== roleId));
        this.success.set(`Role "${roleId}" was removed from this workflow.`);
        setTimeout(() => this.success.set(null), 4000);
      },
      error: err => {
        this.error.set(err.error?.error ?? 'Failed to unmap role.');
        this.isUnmapping.set(false);
        this.showUnmapConfirm.set(false);
      },
    });
  }

  goBack(): void {
    this.router.navigate(['/dashboard/workflows']);
  }
}
