import { Component, signal, inject, OnInit, DestroyRef } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { WorkflowService, Workflow, WorkflowNode } from '../../services/workflow.service';
import { AdminService } from '../../services/admin.service';
import { DagPreviewComponent } from './dag-preview/dag-preview.component';
import type { FormNode } from './dag-preview/dag-preview.component';

// Re-export so template can use the type indirectly.
export type { FormNode };

interface BuilderRole {
  id?: string;
  name: string;
  description?: string;
}

@Component({
  selector: 'app-workflow-builder',
  imports: [FormsModule, RouterLink, DagPreviewComponent],
  templateUrl: './workflow-builder.component.html',
  styleUrl: './workflow-builder.component.scss'
})
export class WorkflowBuilderComponent implements OnInit {
  private readonly workflowService = inject(WorkflowService);
  private readonly adminService = inject(AdminService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);

  /** 'template' when creating a standalone template, 'role' when configuring for a specific role. */
  readonly builderMode = signal<'template' | 'role'>('role');
  readonly roleId = signal('');
  readonly workflowName = signal('');
  readonly workflowDescription = signal('');
  readonly nodes = signal<FormNode[]>([]);

  readonly roles = signal<BuilderRole[]>([]);
  readonly users = signal<any[]>([]);
  readonly activeDropdownId = signal<string | null>(null);
  readonly isLoading = signal(true);
  readonly isSaving = signal(false);
  readonly error = signal<string | null>(null);
  readonly success = signal<string | null>(null);

  readonly workflowSource = signal<'new' | 'existing'>('new');
  readonly existingWorkflows = signal<Workflow[]>([]);
  readonly selectedWorkflowId = signal<string>('');
  readonly isExistingWorkflowLoaded = signal<boolean>(false);

  ngOnInit(): void {
    const id = this.route.snapshot.paramMap.get('roleId') ?? '';
    const fromId = this.route.snapshot.queryParamMap.get('from') || this.route.snapshot.queryParamMap.get('templateId') || '';

    if (id === 'new' || !id) {
      this.builderMode.set('template');
      if (fromId) {
        this.workflowSource.set('existing');
        this.loadSelectedWorkflowStructure(fromId);
      } else {
        this.workflowName.set('');
        this.nodes.set([this.createDefaultNode('node_1')]);
        this.isLoading.set(false);
      }
    } else {
      this.builderMode.set('role');
      this.roleId.set(id);
      if (fromId) {
        this.workflowSource.set('existing');
        this.loadSelectedWorkflowStructure(fromId);
      } else {
        this.loadWorkflow();
      }
    }

    this.loadRoles();
    this.loadExistingDefinitions();
    this.loadUsers();
  }

  loadRoles(): void {
    this.adminService.listRoles()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (data) => this.roles.set(data ?? []),
      });
  }

  loadExistingDefinitions(): void {
    this.workflowService.listWorkflowTemplates()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (defs) => {
          const uniqueMap = new Map<string, Workflow>();
          for (const def of defs) {
            const key = def.definition_key;
            const existing = uniqueMap.get(key);
            if (!existing || def.version > existing.version) {
              uniqueMap.set(key, def);
            }
          }
          const roleIdLower = this.roleId().toLowerCase();
          const list = Array.from(uniqueMap.values()).filter(d =>
            !roleIdLower || !d.definition_key.toLowerCase().startsWith(roleIdLower)
          );
          this.existingWorkflows.set(list);
        }
      });
  }

  loadUsers(): void {
    this.adminService.listUsers()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (data) => this.users.set(data ?? []),
        error: () => this.error.set('Failed to load users list.')
      });
  }

  setActiveDropdown(nodeIdx: number, candidateIdx: number): void {
    this.activeDropdownId.set(`${nodeIdx}-${candidateIdx}`);
  }

  clearActiveDropdown(): void {
    setTimeout(() => {
      this.activeDropdownId.set(null);
    }, 150);
  }

  isDropdownActive(nodeIdx: number, candidateIdx: number): boolean {
    return this.activeDropdownId() === `${nodeIdx}-${candidateIdx}`;
  }

  getFilteredUsers(query: string): any[] {
    if (!query) {
      return this.users();
    }
    const q = query.toLowerCase();
    return this.users().filter(u =>
      (u.username || '').toLowerCase().includes(q) ||
      (u.first_name || '').toLowerCase().includes(q) ||
      (u.last_name || '').toLowerCase().includes(q) ||
      (u.email || '').toLowerCase().includes(q)
    );
  }

  selectUser(candidate: { type: 'USER' | 'ROLE'; value: string }, username: string): void {
    candidate.value = username;
    this.activeDropdownId.set(null);
  }

  onWorkflowSourceChange(source: 'new' | 'existing'): void {
    this.workflowSource.set(source);
    this.error.set(null);
    this.success.set(null);
    if (source === 'new') {
      this.selectedWorkflowId.set('');
      this.isExistingWorkflowLoaded.set(false);
      if (this.builderMode() === 'role') {
        this.loadWorkflow();
      } else {
        this.workflowName.set('');
        this.nodes.set([this.createDefaultNode('node_1')]);
      }
    }
  }

  loadSelectedWorkflowStructure(id: string): void {
    if (!id) {
      this.selectedWorkflowId.set('');
      this.isExistingWorkflowLoaded.set(false);
      return;
    }
    this.selectedWorkflowId.set(id);
    this.isLoading.set(true);
    this.error.set(null);

    this.workflowService.getWorkflowDefinitionById(id)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (def) => {
          if (def && def.nodes && def.nodes.length > 0) {
            this.workflowName.set(this.builderMode() === 'role' ? `Workflow for ${this.roleId()}` : def.name);
            this.workflowDescription.set(def.description ?? '');
            this.nodes.set(this.mapNodesToForm(def.nodes));
            this.isExistingWorkflowLoaded.set(true);
          } else {
            this.error.set('Selected workflow contains no steps.');
          }
          this.isLoading.set(false);
        },
        error: () => {
          this.error.set('Failed to load selected workflow structure.');
          this.isLoading.set(false);
        }
      });
  }

  loadWorkflow(): void {
    if (!this.roleId()) {
      this.isLoading.set(false);
      return;
    }
    this.isLoading.set(true);
    this.workflowService.getWorkflowDefinition(this.roleId())
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (def) => {
          if (def && def.nodes && def.nodes.length > 0) {
            this.workflowName.set(def.name);
            this.workflowDescription.set(def.description ?? '');
            this.nodes.set(this.mapNodesToForm(def.nodes));
          } else {
            this.workflowName.set(`Workflow for ${this.roleId()}`);
            this.nodes.set([this.createDefaultNode('node_1')]);
          }
          this.isLoading.set(false);
        },
        error: () => {
          this.error.set('Failed to load workflow configuration.');
          this.isLoading.set(false);
        }
      });
  }

  private mapNodesToForm(apiNodes: WorkflowNode[]): FormNode[] {
    return apiNodes.map(n => {
      const config = (n.config || {}) as {
        policy?: 'ANY' | 'ALL' | 'QUORUM';
        min_approvals?: number;
        candidates?: { type?: 'USER' | 'ROLE'; value?: string }[];
      };
      const policy = config.policy || 'ANY';
      const minApprovals = config.min_approvals || 1;
      const candidates = (config.candidates || []).map(c => ({
        type: c.type || 'ROLE',
        value: c.value || ''
      }));
      return {
        id: n.id,
        name: n.name,
        type: n.type || 'APPROVAL',
        depends_on: n.depends_on || [],
        candidates,
        policy,
        min_approvals: minApprovals,
        max_retries: n.type === 'OPERATION' ? (n.max_retries || 0) : 0,
        retry_interval: n.type === 'OPERATION' ? (n.retry_interval_seconds || n.retry_interval || 0) : 0
      } satisfies FormNode;
    });
  }

  createDefaultNode(id: string): FormNode {
    return {
      id,
      name: `Step ${id.replace('node_', '')}`,
      type: 'APPROVAL',
      depends_on: [],
      candidates: [{ type: 'ROLE', value: '' }],
      policy: 'ANY',
      min_approvals: 1,
      max_retries: 0,
      retry_interval: 0
    };
  }

  onNodeNameChange(index: number, newName: string): void {
    const current = this.nodes();
    const node = current[index];
    const oldId = node.id;

    let newId = this.slugify(newName);
    if (!newId) newId = `node_${index + 1}`;

    let finalId = newId;
    let suffix = 1;
    while (current.some((n, i) => i !== index && n.id === finalId)) {
      finalId = `${newId}_${suffix}`;
      suffix++;
    }

    const updatedNodes = current.map((n, i) => {
      if (i === index) {
        return {
          ...n,
          name: newName,
          id: finalId
        };
      }
      if (oldId !== finalId && n.depends_on.includes(oldId)) {
        return {
          ...n,
          depends_on: n.depends_on.map(dep => dep === oldId ? finalId : dep)
        };
      }
      return n;
    });

    this.nodes.set(updatedNodes);
  }

  onNodeTypeChange(index: number, type: 'APPROVAL' | 'OPERATION'): void {
    const updated = this.nodes().map((node, i) => {
      if (i === index) {
        return {
          ...node,
          type,
          max_retries: type === 'APPROVAL' ? 0 : node.max_retries,
          retry_interval: type === 'APPROVAL' ? 0 : node.retry_interval
        };
      }
      return node;
    });
    this.nodes.set(updated);
  }

  private slugify(text: string): string {
    return text
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9_]+/g, '_')
      .replace(/^_+|_+$/g, '');
  }

  addNode(): void {
    const current = this.nodes();
    const nextId = `node_${current.length + 1}`;
    this.nodes.set([...current, this.createDefaultNode(nextId)]);
  }

  removeNode(index: number): void {
    const current = this.nodes();
    if (current.length <= 1) return;
    const nodeToRemove = current[index];
    const updated = current
      .filter((_, i) => i !== index)
      .map(n => ({
        ...n,
        depends_on: n.depends_on.filter(dep => dep !== nodeToRemove.id)
      }));
    this.nodes.set(updated);
  }

  addCandidate(nodeIndex: number): void {
    const updated = this.nodes().map((node, i) => {
      if (i === nodeIndex) {
        return {
          ...node,
          candidates: [...node.candidates, { type: 'ROLE' as const, value: '' }]
        };
      }
      return node;
    });
    this.nodes.set(updated);
  }

  removeCandidate(nodeIndex: number, candidateIndex: number): void {
    const updated = this.nodes().map((node, i) => {
      if (i === nodeIndex) {
        if (node.candidates.length <= 1) return node;
        return {
          ...node,
          candidates: node.candidates.filter((_, cIdx) => cIdx !== candidateIndex)
        };
      }
      return node;
    });
    this.nodes.set(updated);
  }

  isAncestor(ancestorId: string, descendentId: string, nodes: FormNode[] = this.nodes()): boolean {
    const visited = new Set<string>();
    const queue = [descendentId];
    while (queue.length > 0) {
      const currId = queue.shift()!;
      if (currId === ancestorId) return true;
      if (visited.has(currId)) continue;
      visited.add(currId);
      const currNode = nodes.find(n => n.id === currId);
      if (currNode) {
        queue.push(...currNode.depends_on);
      }
    }
    return false;
  }

  isDependencyDisabled(nodeIndex: number, otherNodeId: string): boolean {
    const current = this.nodes();
    const node = current[nodeIndex];

    if (node.depends_on.includes(otherNodeId)) {
      return false;
    }

    if (this.isAncestor(node.id, otherNodeId, current)) {
      return true;
    }

    for (const depId of node.depends_on) {
      if (this.isAncestor(depId, otherNodeId, current) || this.isAncestor(otherNodeId, depId, current)) {
        return true;
      }
    }

    return false;
  }

  private hasCycle(nodes: FormNode[]): boolean {
    const WHITE = 0, GRAY = 1, BLACK = 2;
    const color = new Map<string, number>(nodes.map(n => [n.id, WHITE]));

    const dfs = (id: string): boolean => {
      color.set(id, GRAY);
      const node = nodes.find(n => n.id === id);
      if (!node) { color.set(id, BLACK); return false; }
      for (const dep of node.depends_on) {
        const c = color.get(dep) ?? BLACK;
        if (c === GRAY) return true;
        if (c === WHITE && dfs(dep)) return true;
      }
      color.set(id, BLACK);
      return false;
    };

    for (const node of nodes) {
      if ((color.get(node.id) ?? WHITE) === WHITE) {
        if (dfs(node.id)) return true;
      }
    }
    return false;
  }

  toggleDependency(nodeIndex: number, depId: string): void {
    const current = this.nodes();
    const node = current[nodeIndex];

    let updatedDependsOn: string[];
    if (node.depends_on.includes(depId)) {
      updatedDependsOn = node.depends_on.filter(id => id !== depId);
      this.error.set(null);
    } else {
      if (this.isDependencyDisabled(nodeIndex, depId)) {
        if (this.isAncestor(node.id, depId, current)) {
          this.error.set(
            `Cannot add dependency: "${node.name}" depends on "${current.find(n => n.id === depId)?.name ?? depId
            }" (creating a cycle).`
          );
        } else {
          this.error.set(
            `Cannot add dependency: "${current.find(n => n.id === depId)?.name ?? depId
            }" has a dependency relationship with an existing predecessor of "${node.name}".`
          );
        }
        return;
      }
      updatedDependsOn = [...node.depends_on, depId];
      this.error.set(null);
    }

    const updated = current.map((n, i) => {
      if (i === nodeIndex) {
        return {
          ...n,
          depends_on: updatedDependsOn
        };
      }
      return n;
    });
    this.nodes.set(updated);
  }

  saveWorkflow(): void {
    this.error.set(null);
    this.success.set(null);

    const name = this.workflowName().trim();
    if (!name) {
      this.error.set('Workflow name is required.');
      return;
    }

    if (this.hasCycle(this.nodes())) {
      this.error.set('The workflow graph contains a cyclic dependency. Please resolve all cycles before saving.');
      return;
    }

    const current = this.nodes();
    for (const node of current) {
      const deps = node.depends_on;
      for (let j = 0; j < deps.length; j++) {
        for (let k = j + 1; k < deps.length; k++) {
          if (this.isAncestor(deps[j], deps[k], current) || this.isAncestor(deps[k], deps[j], current)) {
            const nameJ = current.find(n => n.id === deps[j])?.name ?? deps[j];
            const nameK = current.find(n => n.id === deps[k])?.name ?? deps[k];
            this.error.set(`Cannot save: In step "${node.name}", predecessors "${nameJ}" and "${nameK}" are dependent on each other.`);
            return;
          }
        }
      }
    }

    const currentNodes = this.nodes();
    for (let i = 0; i < currentNodes.length; i++) {
      const n = currentNodes[i];
      if (!n.id.trim()) { this.error.set(`Node ${i + 1} ID is required.`); return; }
      if (n.id.includes(' ')) { this.error.set(`Node ID "${n.id}" must not contain spaces.`); return; }
      if (!n.name.trim()) { this.error.set(`Node "${n.id}" name is required.`); return; }
      if (n.type === 'APPROVAL') {
        if (n.candidates.length === 0) { this.error.set(`Node "${n.name}" must have at least one candidate.`); return; }
        for (let cIdx = 0; cIdx < n.candidates.length; cIdx++) {
          const candidate = n.candidates[cIdx];
          if (!candidate.value.trim()) {
            this.error.set(`Node "${n.name}", Candidate ${cIdx + 1} is missing target user/role value.`); return;
          }
          if (candidate.type === 'USER') {
            const val = candidate.value.trim();
            if (!this.users().some(u => u.username === val)) {
              this.error.set(`Node "${n.name}", Candidate ${cIdx + 1}: User "${val}" does not exist.`); return;
            }
          }
        }
      }
    }

    this.isSaving.set(true);

    const payload = {
      name,
      description: this.workflowDescription().trim(),
      nodes: currentNodes.map(n => ({
        id: n.id.trim(),
        name: n.name.trim(),
        type: n.type,
        depends_on: n.depends_on,
        max_retries: n.type === 'OPERATION' ? (n.max_retries || 0) : 0,
        retry_interval_seconds: n.type === 'OPERATION' ? (n.retry_interval || 0) : 0,
        config: n.type === 'APPROVAL' ? {
          policy: n.policy,
          min_approvals: n.policy === 'QUORUM' ? n.min_approvals : undefined,
          candidates: n.candidates.map(c => ({ type: c.type, value: c.value.trim() }))
        } : {}
      }))
    };

    if (this.builderMode() === 'template') {
      this.workflowService.createWorkflowTemplate(payload)
        .pipe(takeUntilDestroyed(this.destroyRef))
        .subscribe({
          next: () => {
            this.success.set('Workflow template created successfully.');
            this.isSaving.set(false);
            setTimeout(() => this.router.navigate(['/dashboard/workflows']), 1500);
          },
          error: (err) => {
            this.error.set(err.error?.error ?? 'Failed to save template.');
            this.isSaving.set(false);
          }
        });
    } else {
      this.workflowService.upsertWorkflowDefinition(this.roleId(), payload)
        .pipe(takeUntilDestroyed(this.destroyRef))
        .subscribe({
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
}
