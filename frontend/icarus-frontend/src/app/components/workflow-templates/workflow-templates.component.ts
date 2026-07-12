import { Component, signal, inject, OnInit } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { forkJoin } from 'rxjs';
import { WorkflowService, WorkflowWithRoleMapping } from '../../services/workflow.service';
import { AdminService } from '../../services/admin.service';


@Component({
  selector: 'app-workflow-templates',
  imports: [RouterLink, FormsModule],
  templateUrl: './workflow-templates.component.html',
  styleUrl: './workflow-templates.component.scss',
})

export class WorkflowTemplatesComponent implements OnInit {
  private readonly workflowService = inject(WorkflowService);
  private readonly adminService = inject(AdminService);
  private readonly router = inject(Router);

  readonly templates = signal<WorkflowWithRoleMapping[]>([]);
  readonly roles = signal<any[]>([]);
  readonly isLoading = signal(true);
  readonly error = signal<string | null>(null);
  readonly success = signal<string | null>(null);

  // Map modal state
  readonly showMapModal = signal(false);
  readonly mappingTemplateId = signal('');
  readonly mappingTemplateNme = signal('');
  readonly selectedRoleIds = signal<Set<string>>(new Set());
  /** Snapshot of roles mapped when the modal was opened — used to diff additions vs removals. */
  private originalRoleIds = new Set<string>();
  readonly isMapping = signal(false);

  ngOnInit(): void {
    this.loadTemplates();
    this.loadRoles();
  }

  loadTemplates(): void {
    this.isLoading.set(true);
    this.workflowService.listWorkflowTemplates().subscribe({
      next: (list) => {
        this.templates.set(list ?? []);
        this.isLoading.set(false);
      },
      error: () => {
        this.error.set('Failed to load workflow templates.');
        this.isLoading.set(false);
      }
    });
  }

  loadRoles(): void {
    this.adminService.listRoles().subscribe({
      next: (roles) => this.roles.set(roles ?? []),
    });
  }

  openMapModal(template: WorkflowWithRoleMapping): void {
    this.mappingTemplateId.set(template.id);
    this.mappingTemplateNme.set(template.name);
    const existing = new Set(template.mapped_role_ids ?? []);
    this.originalRoleIds = new Set(existing); // snapshot for diffing
    this.selectedRoleIds.set(new Set(existing));
    this.showMapModal.set(true);
    this.error.set(null);
    this.success.set(null);
  }

  closeMapModal(): void {
    this.showMapModal.set(false);
    this.selectedRoleIds.set(new Set());
    this.originalRoleIds = new Set();
  }

  toggleRole(roleId: string): void {
    const current = new Set(this.selectedRoleIds());
    if (current.has(roleId)) {
      current.delete(roleId);
    } else {
      current.add(roleId);
    }
    this.selectedRoleIds.set(current);
  }

  isRoleSelected(roleId: string): boolean {
    return this.selectedRoleIds().has(roleId);
  }

  confirmMap(): void {
    const selected = this.selectedRoleIds();
    const workflowId = this.mappingTemplateId();

    // Diff: roles to add (newly checked) and roles to remove (previously mapped but now unchecked)
    const toMap = [...selected].filter(id => !this.originalRoleIds.has(id));
    const toUnmap = [...this.originalRoleIds].filter(id => !selected.has(id));

    if (toMap.length === 0 && toUnmap.length === 0) {
      this.closeMapModal();
      return;
    }

    this.isMapping.set(true);
    const ops = [
      ...toMap.map(id => this.workflowService.mapRoleToWorkflow(workflowId, id)),
      ...toUnmap.map(id => this.workflowService.unmapRoleFromWorkflow(workflowId, id)),
    ];

    forkJoin(ops).subscribe({
      next: () => {
        this.isMapping.set(false);
        this.closeMapModal();
        const parts: string[] = [];
        if (toMap.length) parts.push(`mapped ${toMap.length} role${toMap.length > 1 ? 's' : ''}`);
        if (toUnmap.length) parts.push(`unmapped ${toUnmap.length} role${toUnmap.length > 1 ? 's' : ''}`);
        this.success.set(`Workflow template: ${parts.join(' and ')} successfully.`);
        this.loadTemplates();
        setTimeout(() => this.success.set(null), 4000);
      },
      error: (err) => {
        this.error.set(err.error?.error ?? 'Failed to update role mappings.');
        this.isMapping.set(false);
      }
    });
  }

  /** Directly unmaps a single role from a workflow card pill (no modal needed). */
  unmapRole(workflowId: string, roleId: string): void {
    this.workflowService.unmapRoleFromWorkflow(workflowId, roleId).subscribe({
      next: () => {
        this.success.set(`Role "${roleId}" unmapped successfully.`);
        this.loadTemplates();
        setTimeout(() => this.success.set(null), 4000);
      },
      error: (err) => this.error.set(err.error?.error ?? 'Failed to unmap role.')
    });
  }

  navigateToBuilder(): void {
    this.router.navigate(['/dashboard/workflow-builder/new']);
  }

  navigateToEdit(template: WorkflowWithRoleMapping): void {
    // Edit by loading via its role mapping if exists, else go to a template-specific route.
    if (template.mapped_role_ids && template.mapped_role_ids.length > 0) {
      this.router.navigate(['/dashboard/workflow-builder', template.mapped_role_ids[0]]);
    } else {
      this.router.navigate(['/dashboard/workflow-builder/new']);
    }
  }

  nodeCountLabel(template: WorkflowWithRoleMapping): string {
    const count = template.nodes?.length ?? 0;
    return `${count} node${count === 1 ? '' : 's'}`;
  }

  statusClass(status: string): string {
    if (status === 'ACTIVE') return 'status-active';
    if (status === 'SUPERSEDED') return 'status-superseded';
    return 'status-draft';
  }
}
