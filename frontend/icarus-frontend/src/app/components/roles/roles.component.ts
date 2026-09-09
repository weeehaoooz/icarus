import { Component, signal, computed, inject, OnInit, viewChild, TemplateRef } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { AdminService } from '../../services/admin.service';
import { PlatformService } from '../../services/platform.service';

// Talos UI
import { TalosDataGridComponent, type TalosGridColDef } from '@weeehaoooz/talos-ui/data-display/data-grid';
import { TalosFormFieldComponent } from '@weeehaoooz/talos-ui/form/form-field';
import { TalosPrefixDirective, TalosSuffixDirective } from '@weeehaoooz/talos-ui/form/affix';
import { TalosInputDirective } from '@weeehaoooz/talos-ui/form/input';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';
import { SelectInputComponent, OptionComponent } from '@weeehaoooz/talos-ui/form/select-input';
import { TalosSlideToggleComponent } from '@weeehaoooz/talos-ui/form/slide-toggle';
import { TalosCheckboxDirective } from '@weeehaoooz/talos-ui/form/checkbox';

// Lucide Icons
import { LucideSearch, LucideX, LucidePlus, LucideGitBranch, LucideTrash2 } from '@lucide/angular';

interface Permission {
  id: string;
  module_id: string;
  action: string;
  description: string;
}

interface Role {
  id?: string;
  module_id: string;
  app_code?: string;
  name: string;
  description: string;
  is_active?: boolean;
  permissions: string[];
  nested_roles?: string[];
  type?: 'LDAP' | 'Custom';
}

@Component({
  selector: 'app-roles',
  imports: [
    FormsModule,
    RouterLink,
    TalosDataGridComponent,
    TalosFormFieldComponent,
    TalosPrefixDirective,
    TalosSuffixDirective,
    TalosInputDirective,
    TalosButtonDirective,
    TalosAlertComponent,
    SelectInputComponent,
    OptionComponent,
    TalosSlideToggleComponent,
    TalosCheckboxDirective,
    LucideSearch,
    LucideX,
    LucidePlus,
    LucideGitBranch,
    LucideTrash2
  ],
  templateUrl: './roles.component.html',
  styleUrl: './roles.component.scss'
})
export class RolesComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly platformService = inject(PlatformService);

  // Template Refs
  readonly nameTmpl = viewChild<TemplateRef<any>>('nameTmpl');
  readonly moduleTmpl = viewChild<TemplateRef<any>>('moduleTmpl');
  readonly appScopeTmpl = viewChild<TemplateRef<any>>('appScopeTmpl');
  readonly typeTmpl = viewChild<TemplateRef<any>>('typeTmpl');
  readonly nestedRolesTmpl = viewChild<TemplateRef<any>>('nestedRolesTmpl');
  readonly permissionsTmpl = viewChild<TemplateRef<any>>('permissionsTmpl');
  readonly actionsTmpl = viewChild<TemplateRef<any>>('actionsTmpl');

  // Data Signals
  readonly roles = signal<Role[]>([]);
  readonly availablePermissions = signal<Permission[]>([]);
  readonly modules = signal<any[]>([]);
  readonly applications = signal<any[]>([]);
  readonly searchQuery = signal('');
  readonly isLoading = signal(false);

  // Selected module in form (drives filtered permissions)
  readonly selectedFormModuleId = signal<string>('auth-ms');

  // Filter permissions based on the selected module in the form
  readonly filteredPermissions = computed(() => {
    const modId = this.selectedFormModuleId();
    return this.availablePermissions().filter(p => p.module_id === modId);
  });

  // Filtered Roles list
  readonly filteredRoles = computed(() => {
    const query = this.searchQuery().toLowerCase().trim();
    const allRoles = this.roles();
    if (!query) return allRoles;
    return allRoles.filter(r =>
      r.name.toLowerCase().includes(query) ||
      (r.module_id && r.module_id.toLowerCase().includes(query)) ||
      (r.app_code && r.app_code.toLowerCase().includes(query)) ||
      (r.description && r.description.toLowerCase().includes(query)) ||
      (r.type && r.type.toLowerCase().includes(query))
    );
  });

  // Columns definition for talos-data-grid
  readonly columns = computed<TalosGridColDef<Role>[]>(() => [
    { field: 'name', header: 'Role Name', sortType: 'string', filterMode: 'set', minWidth: '180px', cellTemplate: this.nameTmpl() },
    { field: 'module_id', header: 'Module', sortType: 'string', filterMode: 'set', width: '140px', cellTemplate: this.moduleTmpl() },
    { field: 'app_code', header: 'App Scope', sortType: 'string', filterMode: 'set', width: '130px', cellTemplate: this.appScopeTmpl() },
    { field: 'type', header: 'Type', sortType: 'string', filterMode: 'set', width: '110px', cellTemplate: this.typeTmpl() },
    { field: 'description', header: 'Description', sortType: 'string', filterMode: 'set', minWidth: '200px', formatter: (v: any) => v || 'No description provided.' },
    { field: 'nested_roles', header: 'Nested Roles', minWidth: '160px', cellTemplate: this.nestedRolesTmpl() },
    { field: 'permissions', header: 'Permissions Assigned', width: '160px', cellTemplate: this.permissionsTmpl() },
    { field: 'actions', header: 'Actions', width: '100px', minWidth: '100px', align: 'right', sortable: false, filterable: false, cellTemplate: this.actionsTmpl() }
  ]);

  // Panel Signals (replaces modal)
  readonly activePanel = signal<'create' | 'edit' | null>(null);
  readonly panelError = signal<string | null>(null);
  readonly selectedRole = signal<Role | null>(null);

  // Form State
  formData = {
    name: '',
    description: '',
    permissions: [] as string[],
    nested_roles: [] as string[],
    type: 'Custom' as 'LDAP' | 'Custom',
    module_id: 'auth-ms',
    app_code: '',
    is_active: true
  };

  readonly currentRoleName = signal<string>('');

  // Find all ancestor roles of the current role to prevent cycle creation
  readonly ancestorRoles = computed(() => {
    const currentName = this.currentRoleName();
    if (!currentName) return new Set<string>();

    const allRoles = this.roles();
    const ancestors = new Set<string>();

    const visit = (roleName: string) => {
      for (const r of allRoles) {
        if ((r.nested_roles || []).includes(roleName)) {
          if (!ancestors.has(r.name)) {
            ancestors.add(r.name);
            visit(r.name);
          }
        }
      }
    };

    visit(currentName);
    return ancestors;
  });

  // Filter out the current role and its ancestors from potential nested roles
  readonly availableRolesToNest = computed(() => {
    const currentName = this.currentRoleName();
    const ancestors = this.ancestorRoles();
    return this.roles().filter(r => r.name !== currentName && !ancestors.has(r.name));
  });

  ngOnInit(): void {
    this.loadData();
  }

  loadData(): void {
    this.isLoading.set(true);
    this.adminService.listRoles().subscribe({
      next: (data) => {
        this.roles.set(data as Role[]);
        this.isLoading.set(false);
      },
      error: (err) => {
        console.error('Failed to load roles:', err);
        this.isLoading.set(false);
      }
    });

    this.adminService.listPermissions().subscribe({
      next: (data) => this.availablePermissions.set(data as Permission[]),
      error: (err) => console.error('Failed to load permissions:', err)
    });

    this.platformService.listModules().subscribe({
      next: (data) => this.modules.set(data),
      error: (err) => console.error('Failed to load modules:', err)
    });

    this.platformService.listApplications().subscribe({
      next: (data) => this.applications.set(data),
      error: (err) => console.error('Failed to load applications:', err)
    });
  }

  onRowClick(event: { row: Role; index: number }): void {
    this.openEditPanel(event.row);
  }

  onModuleChange(eventOrValue: any): void {
    const value = typeof eventOrValue === 'string' 
      ? eventOrValue 
      : (eventOrValue?.target ? (eventOrValue.target as HTMLSelectElement).value : eventOrValue);
    if (value) {
      this.selectedFormModuleId.set(value);
      this.formData.module_id = value;
      this.formData.permissions = [];
    }
  }

  openCreatePanel(): void {
    this.selectedRole.set(null);
    this.panelError.set(null);
    this.currentRoleName.set('');
    this.selectedFormModuleId.set('auth-ms');
    this.formData = {
      name: '',
      description: '',
      permissions: [],
      nested_roles: [],
      type: 'Custom',
      module_id: 'auth-ms',
      app_code: '',
      is_active: true
    };
    this.activePanel.set('create');
  }

  openEditPanel(role: Role): void {
    this.selectedRole.set(role);
    this.panelError.set(null);
    this.currentRoleName.set(role.name);
    this.selectedFormModuleId.set(role.module_id || 'auth-ms');
    this.formData = {
      name: role.name,
      description: role.description,
      permissions: role.permissions ? [...role.permissions] : [],
      nested_roles: role.nested_roles ? [...role.nested_roles] : [],
      type: role.type || 'Custom',
      module_id: role.module_id || 'auth-ms',
      app_code: role.app_code || '',
      is_active: role.is_active !== false
    };
    this.activePanel.set('edit');
  }

  closePanel(): void {
    this.activePanel.set(null);
    this.selectedRole.set(null);
  }

  togglePermission(permName: string): void {
    const currentPerms = this.formData.permissions;
    if (currentPerms.includes(permName)) {
      this.formData.permissions = currentPerms.filter(p => p !== permName);
    } else {
      this.formData.permissions = [...currentPerms, permName];
    }
  }

  isPermissionSelected(permName: string): boolean {
    return this.formData.permissions.includes(permName);
  }

  toggleNestedRole(roleName: string): void {
    const currentNested = this.formData.nested_roles;
    if (currentNested.includes(roleName)) {
      this.formData.nested_roles = currentNested.filter(r => r !== roleName);
    } else {
      this.formData.nested_roles = [...currentNested, roleName];
    }
  }

  isRoleNestedSelected(roleName: string): boolean {
    return this.formData.nested_roles.includes(roleName);
  }

  saveRole(): void {
    this.panelError.set(null);

    const payload = {
      name: this.formData.name,
      description: this.formData.description,
      permissions: this.formData.permissions,
      nested_roles: this.formData.nested_roles,
      type: this.formData.type,
      module_id: this.formData.module_id,
      app_code: this.formData.app_code,
      is_active: this.formData.is_active
    };

    if (this.activePanel() === 'edit') {
      const roleID = this.selectedRole()?.id || this.formData.name;
      this.adminService.updateRole(roleID, {
        description: this.formData.description,
        permissions: this.formData.permissions,
        nested_roles: this.formData.nested_roles,
        type: this.formData.type,
        module_id: this.formData.module_id,
        app_code: this.formData.app_code,
        is_active: this.formData.is_active
      }).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to update role policy.');
        }
      });
    } else {
      this.adminService.createRole(payload).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to create access role.');
        }
      });
    }
  }

  deleteRole(role: Role, event: Event): void {
    event.stopPropagation();
    if (role.name === 'admin' && role.module_id === 'auth-ms') {
      alert('The admin role is system protected and cannot be deleted.');
      return;
    }
    const roleID = role.id || role.name;
    if (confirm(`Are you sure you want to delete the role '${role.name}'?`)) {
      this.adminService.deleteRole(roleID).subscribe({
        next: () => {
          if (this.selectedRole()?.id === role.id) {
            this.closePanel();
          }
          this.loadData();
        },
        error: (err) => alert(err.error?.error || 'Failed to delete role.')
      });
    }
  }
}

