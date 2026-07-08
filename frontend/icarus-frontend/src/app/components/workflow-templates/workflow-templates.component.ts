import { Component, signal, inject, OnInit, computed } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { WorkflowService, WorkflowWithRoleMapping, Workflow } from '../../services/workflow.service';
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
  readonly selectedRoleId = signal('');
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
    this.selectedRoleId.set(template.mapped_role_id ?? '');
    this.showMapModal.set(true);
    this.error.set(null);
    this.success.set(null);
  }

  closeMapModal(): void {
    this.showMapModal.set(false);
    this.selectedRoleId.set('');
  }

  confirmMap(): void {
    const roleId = this.selectedRoleId();
    const workflowId = this.mappingTemplateId();
    if (!roleId) {
      this.error.set('Please select a role to map to.');
      return;
    }

    this.isMapping.set(true);
    this.workflowService.mapWorkflowToRole(roleId, workflowId).subscribe({
      next: () => {
        this.isMapping.set(false);
        this.closeMapModal();
        this.success.set(`Workflow template mapped to role "${roleId}" successfully.`);
        this.loadTemplates();
        setTimeout(() => this.success.set(null), 4000);
      },
      error: (err) => {
        this.error.set(err.error?.error ?? 'Failed to map workflow to role.');
        this.isMapping.set(false);
      }
    });
  }

  navigateToBuilder(): void {
    this.router.navigate(['/dashboard/workflow-builder/new']);
  }

  navigateToEdit(template: WorkflowWithRoleMapping): void {
    // Edit by loading via its role mapping if exists, else go to a template-specific route.
    if (template.mapped_role_id) {
      this.router.navigate(['/dashboard/workflow-builder', template.mapped_role_id]);
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
