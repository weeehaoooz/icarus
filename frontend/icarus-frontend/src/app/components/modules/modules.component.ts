import { Component, signal, computed, inject, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { PlatformService } from '../../services/platform.service';
import { AdminService } from '../../services/admin.service';
import { forkJoin } from 'rxjs';

interface Module {
  id: string;
  code: string;
  name: string;
  base_url: string;
  is_active: boolean;
  created_at: string;
}

export interface PermissionEntry {
  action: string;
  path_pattern: string;
  method: string;
  description: string;
}

export interface RoleEntry {
  name: string;
  description: string;
  permissions: string[];
}

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'HEAD'];

@Component({
  selector: 'app-modules',
  imports: [FormsModule, DatePipe],
  templateUrl: './modules.component.html',
  styleUrl: './modules.component.scss'
})
export class ModulesComponent implements OnInit {
  private readonly platformService = inject(PlatformService);
  private readonly adminService = inject(AdminService);

  readonly httpMethods = HTTP_METHODS;

  // Data Signals
  readonly modules = signal<Module[]>([]);
  readonly systemRoles = signal<any[]>([]);
  readonly applications = signal<any[]>([]);
  readonly onboardedApps = signal<string[]>([]);
  
  readonly searchQuery = signal('');
  readonly isSearching = signal(false);
  private searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Selection & Bulk Signals
  readonly selectedIds = signal<Set<string>>(new Set());

  // Panel Signals
  readonly activePanel = signal<'create' | 'edit' | null>(null);
  readonly panelError = signal<string | null>(null);
  readonly selectedModule = signal<Module | null>(null);

  // Edit Panel Tabs: 'settings' | 'policies' | 'roles'
  readonly editPanelTab = signal<'settings' | 'policies' | 'roles'>('settings');

  // Config Tab State: 'visual' | 'json'
  readonly activeConfigTab = signal<'visual' | 'json'>('visual');
  readonly configTabError = signal<string | null>(null);

  // Visual sub-tab: 'permissions' | 'default_roles' | 'app_centric_roles'
  readonly activeVisualSubTab = signal<'permissions' | 'default_roles' | 'app_centric_roles'>('default_roles');

  // Visual Structured Models
  readonly permissions = signal<PermissionEntry[]>([]);
  readonly defaultRoles = signal<RoleEntry[]>([]);
  readonly appCentricRoles = signal<RoleEntry[]>([]);

  // New/editing form states
  editingPermIndex = signal<number | null>(null);
  newPermission: PermissionEntry = this.emptyPermission();

  editingDefaultRoleIndex = signal<number | null>(null);
  newDefaultRole: RoleEntry = this.emptyRole();

  editingAppRoleIndex = signal<number | null>(null);
  newAppCentricRole: RoleEntry = this.emptyRole();

  // All defined permission actions (for role checklist)
  readonly permissionActions = computed(() => this.permissions().map(p => p.action));

  // Form State
  formData = {
    id: '',
    code: '',
    name: '',
    baseUrl: '',
    isActive: true
  };
  onboardAppCode = '';
  manifestJson = '';

  // Filtered list
  readonly filteredModules = computed(() => {
    const query = this.searchQuery().toLowerCase().trim();
    const allModules = this.modules() || [];
    if (!query) return allModules;
    return allModules.filter(m => 
      m.name.toLowerCase().includes(query) ||
      m.code.toLowerCase().includes(query) ||
      m.base_url.toLowerCase().includes(query)
    );
  });

  // Filtered Roles under Selected Module
  readonly moduleRoles = computed(() => {
    const selected = this.selectedModule();
    if (!selected) return [];
    return this.systemRoles().filter(r => r.module_id === selected.code && (!r.app_code || r.app_code === ''));
  });

  readonly moduleAppRoles = computed(() => {
    const selected = this.selectedModule();
    if (!selected) return [];
    return this.systemRoles().filter(r => r.module_id === selected.code && r.app_code && r.app_code !== '');
  });

  // Applications available to onboard (not yet onboarded to selected module)
  readonly availableAppsToOnboard = computed(() => {
    const onboarded = new Set(this.onboardedApps());
    return this.applications().filter(app => !onboarded.has(app.code));
  });

  // Is everything selected
  readonly isAllSelected = computed(() => {
    const list = this.filteredModules();
    if (list.length === 0) return false;
    const selected = this.selectedIds();
    return list.every(m => selected.has(m.id));
  });

  private emptyPermission(): PermissionEntry {
    return { action: '', path_pattern: '', method: 'GET', description: '' };
  }

  private emptyRole(): RoleEntry {
    return { name: '', description: '', permissions: [] };
  }

  ngOnInit(): void {
    this.loadData();
  }

  loadData(): void {
    this.platformService.listModules().subscribe({
      next: (data) => this.modules.set((data as Module[]) || []),
      error: (err) => console.error('Failed to load modules:', err)
    });

    this.adminService.listRoles().subscribe({
      next: (data) => this.systemRoles.set(data || []),
      error: (err) => console.error('Failed to load roles:', err)
    });

    this.platformService.listApplications().subscribe({
      next: (data) => this.applications.set(data || []),
      error: (err) => console.error('Failed to load applications:', err)
    });
  }

  onSearchInput(event: Event): void {
    const val = (event.target as HTMLInputElement).value;
    this.searchQuery.set(val);
    this.isSearching.set(true);
    if (this.searchDebounceTimer) clearTimeout(this.searchDebounceTimer);
    this.searchDebounceTimer = setTimeout(() => this.isSearching.set(false), 350);
  }

  clearSearch(): void {
    this.searchQuery.set('');
    this.isSearching.set(false);
    if (this.searchDebounceTimer) clearTimeout(this.searchDebounceTimer);
  }

  // Row selection
  toggleSelect(id: string, event: Event): void {
    event.stopPropagation();
    const current = new Set(this.selectedIds());
    if (current.has(id)) {
      current.delete(id);
    } else {
      current.add(id);
    }
    this.selectedIds.set(current);
  }

  toggleSelectAll(event: Event): void {
    const checked = (event.target as HTMLInputElement).checked;
    const current = new Set(this.selectedIds());
    const list = this.filteredModules();

    if (checked) {
      list.forEach(m => current.add(m.id));
    } else {
      list.forEach(m => current.delete(m.id));
    }
    this.selectedIds.set(current);
  }

  clearSelection(): void {
    this.selectedIds.set(new Set());
  }

  // ─── Config Tab Management ──────────────────────────────────────────────────

  switchConfigTab(tab: 'visual' | 'json'): void {
    if (this.activeConfigTab() === tab) return;
    this.configTabError.set(null);

    if (tab === 'json') {
      // Switching to JSON: serialize visual state into JSON text
      this.syncVisualToJson();
      this.activeConfigTab.set('json');
    } else {
      // Switching to Visual: parse current JSON text into visual state
      const success = this.syncJsonToVisual();
      if (success) {
        this.activeConfigTab.set('visual');
      }
    }
  }

  private syncVisualToJson(): void {
    const manifestObj = {
      permissions: this.permissions(),
      default_roles: this.defaultRoles(),
      app_centric_roles: this.appCentricRoles()
    };
    this.manifestJson = JSON.stringify(manifestObj, null, 2);
  }

  private syncJsonToVisual(): boolean {
    try {
      const parsed = JSON.parse(this.manifestJson);
      this.permissions.set(parsed.permissions || []);
      this.defaultRoles.set(parsed.default_roles || []);
      this.appCentricRoles.set(parsed.app_centric_roles || []);
      this.editingPermIndex.set(null);
      this.editingDefaultRoleIndex.set(null);
      this.editingAppRoleIndex.set(null);
      this.newPermission = this.emptyPermission();
      this.newDefaultRole = this.emptyRole();
      this.newAppCentricRole = this.emptyRole();
      return true;
    } catch {
      this.configTabError.set('Invalid JSON — fix syntax errors before switching to Visual Editor.');
      return false;
    }
  }

  private initVisualFromManifest(manifest: any): void {
    this.permissions.set((manifest.permissions || []).map((p: any) => ({
      action: p.action || '',
      path_pattern: p.path_pattern || '',
      method: p.method || 'GET',
      description: p.description || ''
    })));
    this.defaultRoles.set((manifest.default_roles || []).map((r: any) => ({
      name: r.name || '',
      description: r.description || '',
      permissions: r.permissions || []
    })));
    this.appCentricRoles.set((manifest.app_centric_roles || []).map((r: any) => ({
      name: r.name || '',
      description: r.description || '',
      permissions: r.permissions || []
    })));
    this.editingPermIndex.set(null);
    this.editingDefaultRoleIndex.set(null);
    this.editingAppRoleIndex.set(null);
    this.newPermission = this.emptyPermission();
    this.newDefaultRole = this.emptyRole();
    this.newAppCentricRole = this.emptyRole();
  }

  // ─── Visual Sub-tab ─────────────────────────────────────────────────────────

  switchVisualSubTab(tab: 'permissions' | 'default_roles' | 'app_centric_roles'): void {
    this.activeVisualSubTab.set(tab);
  }

  // ─── Permission CRUD ─────────────────────────────────────────────────────────

  addPermission(): void {
    if (!this.newPermission.action.trim()) return;
    const existing = this.permissions();
    if (existing.some(p => p.action === this.newPermission.action.trim())) {
      this.panelError.set(`Permission action '${this.newPermission.action}' already exists.`);
      return;
    }
    this.panelError.set(null);
    this.permissions.update(list => [...list, { ...this.newPermission, action: this.newPermission.action.trim() }]);
    this.newPermission = this.emptyPermission();
  }

  startEditPermission(index: number): void {
    this.editingPermIndex.set(index);
    this.newPermission = { ...this.permissions()[index] };
  }

  saveEditPermission(): void {
    const idx = this.editingPermIndex();
    if (idx === null || !this.newPermission.action.trim()) return;
    this.permissions.update(list => {
      const updated = [...list];
      updated[idx] = { ...this.newPermission, action: this.newPermission.action.trim() };
      return updated;
    });
    this.editingPermIndex.set(null);
    this.newPermission = this.emptyPermission();
  }

  cancelEditPermission(): void {
    this.editingPermIndex.set(null);
    this.newPermission = this.emptyPermission();
  }

  removePermission(index: number): void {
    const action = this.permissions()[index].action;
    const usedInDefault = this.defaultRoles().some(r => r.permissions.includes(action));
    const usedInApp = this.appCentricRoles().some(r => r.permissions.includes(action));
    if (usedInDefault || usedInApp) {
      if (!confirm(`Permission '${action}' is used in existing roles. Removing it will also remove it from those roles. Proceed?`)) return;
      // Remove from roles
      this.defaultRoles.update(roles => roles.map(r => ({ ...r, permissions: r.permissions.filter(p => p !== action) })));
      this.appCentricRoles.update(roles => roles.map(r => ({ ...r, permissions: r.permissions.filter(p => p !== action) })));
    }
    this.permissions.update(list => list.filter((_, i) => i !== index));
  }

  // ─── Default Role CRUD ──────────────────────────────────────────────────────

  addDefaultRole(): void {
    if (!this.newDefaultRole.name.trim()) return;
    this.panelError.set(null);
    this.defaultRoles.update(list => [...list, { ...this.newDefaultRole, name: this.newDefaultRole.name.trim() }]);
    this.newDefaultRole = this.emptyRole();
  }

  startEditDefaultRole(index: number): void {
    this.editingDefaultRoleIndex.set(index);
    const role = this.defaultRoles()[index];
    this.newDefaultRole = { ...role, permissions: [...role.permissions] };
  }

  saveEditDefaultRole(): void {
    const idx = this.editingDefaultRoleIndex();
    if (idx === null || !this.newDefaultRole.name.trim()) return;
    this.defaultRoles.update(list => {
      const updated = [...list];
      updated[idx] = { ...this.newDefaultRole, name: this.newDefaultRole.name.trim() };
      return updated;
    });
    this.editingDefaultRoleIndex.set(null);
    this.newDefaultRole = this.emptyRole();
  }

  cancelEditDefaultRole(): void {
    this.editingDefaultRoleIndex.set(null);
    this.newDefaultRole = this.emptyRole();
  }

  removeDefaultRole(index: number): void {
    this.defaultRoles.update(list => list.filter((_, i) => i !== index));
  }

  toggleRolePermission(rolePerm: string[], action: string): void {
    const idx = rolePerm.indexOf(action);
    if (idx >= 0) {
      rolePerm.splice(idx, 1);
    } else {
      rolePerm.push(action);
    }
  }

  isPermissionChecked(perms: string[], action: string): boolean {
    return perms.includes(action);
  }

  // ─── App Centric Role CRUD ──────────────────────────────────────────────────

  addAppCentricRole(): void {
    if (!this.newAppCentricRole.name.trim()) return;
    this.panelError.set(null);
    this.appCentricRoles.update(list => [...list, { ...this.newAppCentricRole, name: this.newAppCentricRole.name.trim() }]);
    this.newAppCentricRole = this.emptyRole();
  }

  startEditAppRole(index: number): void {
    this.editingAppRoleIndex.set(index);
    const role = this.appCentricRoles()[index];
    this.newAppCentricRole = { ...role, permissions: [...role.permissions] };
  }

  saveEditAppRole(): void {
    const idx = this.editingAppRoleIndex();
    if (idx === null || !this.newAppCentricRole.name.trim()) return;
    this.appCentricRoles.update(list => {
      const updated = [...list];
      updated[idx] = { ...this.newAppCentricRole, name: this.newAppCentricRole.name.trim() };
      return updated;
    });
    this.editingAppRoleIndex.set(null);
    this.newAppCentricRole = this.emptyRole();
  }

  cancelEditAppRole(): void {
    this.editingAppRoleIndex.set(null);
    this.newAppCentricRole = this.emptyRole();
  }

  removeAppCentricRole(index: number): void {
    this.appCentricRoles.update(list => list.filter((_, i) => i !== index));
  }

  // ─── Panel Logic ─────────────────────────────────────────────────────────────

  openCreatePanel(): void {
    this.selectedModule.set(null);
    this.panelError.set(null);
    this.configTabError.set(null);
    this.onboardedApps.set([]);
    this.formData = { id: '', code: '', name: '', baseUrl: '', isActive: true };
    this.editPanelTab.set('settings');
    this.activeConfigTab.set('visual');
    this.activeVisualSubTab.set('default_roles');

    const defaultManifest = {
      permissions: [
        { action: 'read', path_pattern: '/api/v1/resource', method: 'GET', description: 'Read resource' }
      ],
      default_roles: [
        { name: 'Viewer', description: 'Allows viewing resources', permissions: ['read'] }
      ],
      app_centric_roles: [
        { name: 'AppViewer', description: 'Allows app-scoped viewing of resources', permissions: ['read'] }
      ]
    };
    this.initVisualFromManifest(defaultManifest);
    this.syncVisualToJson();
    this.activePanel.set('create');
  }

  openEditPanel(module: Module): void {
    this.selectedModule.set(module);
    this.panelError.set(null);
    this.configTabError.set(null);
    this.formData = {
      id: module.id,
      code: module.code,
      name: module.name,
      baseUrl: module.base_url,
      isActive: module.is_active
    };
    this.onboardAppCode = '';
    this.manifestJson = '';
    this.editPanelTab.set('settings');
    this.activeConfigTab.set('visual');
    this.activeVisualSubTab.set('default_roles');

    // Load full manifest
    this.platformService.getModuleManifest(module.id).subscribe({
      next: (manifest) => {
        this.initVisualFromManifest(manifest);
        this.syncVisualToJson();
      },
      error: (err) => {
        console.error('Failed to load module manifest:', err);
        const empty = { permissions: [], default_roles: [], app_centric_roles: [] };
        this.initVisualFromManifest(empty);
        this.syncVisualToJson();
      }
    });

    this.loadModuleApplications(module.code);
    this.activePanel.set('edit');
  }

  private loadModuleApplications(moduleCode: string): void {
    this.platformService.listModuleApplications(moduleCode).subscribe({
      next: (data) => this.onboardedApps.set(data || []),
      error: (err) => console.error('Failed to load module applications:', err)
    });
  }

  onboardApplication(): void {
    const selected = this.selectedModule();
    const appCode = this.onboardAppCode;
    if (!selected || !appCode) return;
    this.panelError.set(null);

    this.platformService.onboardApplicationToModule(selected.code, appCode).subscribe({
      next: () => {
        this.onboardAppCode = '';
        this.loadModuleApplications(selected.code);
        this.loadData();
      },
      error: (err) => this.panelError.set(err.error?.error || 'Failed to onboard application.')
    });
  }

  offboardApplication(appCode: string): void {
    const selected = this.selectedModule();
    if (!selected || !appCode) return;
    this.panelError.set(null);

    if (confirm(`Are you sure you want to offboard application '${appCode}'? All app roles for this application will be deactivated.`)) {
      this.platformService.offboardApplicationFromModule(selected.code, appCode).subscribe({
        next: () => {
          this.loadModuleApplications(selected.code);
          this.loadData();
        },
        error: (err) => this.panelError.set(err.error?.error || 'Failed to offboard application.')
      });
    }
  }

  closePanel(): void {
    this.activePanel.set(null);
    this.selectedModule.set(null);
  }

  saveModule(): void {
    this.panelError.set(null);

    let permissions: any[] = [];
    let defaultRoles: any[] = [];
    let appCentricRoles: any[] = [];

    if (this.activeConfigTab() === 'visual') {
      // Use visual state directly
      permissions = this.permissions();
      defaultRoles = this.defaultRoles();
      appCentricRoles = this.appCentricRoles();
    } else {
      // Parse from JSON
      try {
        const manifestObj = JSON.parse(this.manifestJson);
        permissions = manifestObj.permissions || [];
        defaultRoles = manifestObj.default_roles || [];
        appCentricRoles = manifestObj.app_centric_roles || [];
      } catch {
        this.panelError.set('Invalid JSON in Manifest Configuration');
        return;
      }
    }

    const payload = {
      id: this.formData.id || this.formData.code,
      code: this.formData.code,
      name: this.formData.name,
      base_url: this.formData.baseUrl,
      is_active: this.formData.isActive,
      permissions,
      default_roles: defaultRoles,
      app_centric_roles: appCentricRoles
    };

    if (this.activePanel() === 'edit') {
      this.platformService.updateModule(this.formData.id, payload).subscribe({
        next: () => { this.closePanel(); this.loadData(); },
        error: (err) => this.panelError.set(err.error?.error || 'Failed to update module.')
      });
    } else {
      this.platformService.createModule(payload).subscribe({
        next: () => { this.closePanel(); this.loadData(); },
        error: (err) => this.panelError.set(err.error?.error || 'Failed to register module.')
      });
    }
  }

  deleteModule(id: string, event: Event): void {
    event.stopPropagation();
    if (confirm('Are you sure you want to delete this module? All associated permissions, roles, and routing mappings will be removed.')) {
      this.platformService.deleteModule(id).subscribe({
        next: () => {
          const current = new Set(this.selectedIds());
          current.delete(id);
          this.selectedIds.set(current);
          this.loadData();
          if (this.selectedModule()?.id === id) {
            this.closePanel();
          }
        },
        error: (err) => console.error('Failed to delete module:', err)
      });
    }
  }

  deleteSelected(): void {
    const ids = Array.from(this.selectedIds());
    if (ids.length === 0) return;

    if (confirm(`Are you sure you want to delete the ${ids.length} selected modules?`)) {
      const requests = ids.map(id => this.platformService.deleteModule(id));
      forkJoin(requests).subscribe({
        next: () => {
          this.selectedIds.set(new Set());
          this.loadData();
          this.closePanel();
        },
        error: (err) => console.error('Failed to delete selected modules:', err)
      });
    }
  }
}
