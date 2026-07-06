import { Component, signal, computed, inject, OnInit } from '@angular/core';
import { AuthService } from '../../services/auth.service';
import { AdminService } from '../../services/admin.service';

interface UserGroup {
  name: string;
  type: string;
}

interface UserProfile {
  id: number;
  username: string;
  email: string;
  first_name: string;
  last_name: string;
  roles?: string[];
  groups?: UserGroup[];
}

interface Role {
  id?: string;
  module_id: string;
  name: string;
  description: string;
  permissions: string[];
  nested_roles?: string[];
}

interface Permission {
  id: string;
  module_id: string;
  action: string;
  description: string;
}

@Component({
  selector: 'app-my-policies',
  imports: [],
  templateUrl: './my-policies.component.html',
  styleUrl: './my-policies.component.scss'
})
export class MyPoliciesComponent implements OnInit {
  private readonly authService = inject(AuthService);
  private readonly adminService = inject(AdminService);

  readonly userProfile = signal<UserProfile | null>(null);
  readonly isLoadingProfile = signal(false);
  readonly profileError = signal<string | null>(null);

  readonly roles = signal<Role[]>([]);
  readonly permissions = signal<Permission[]>([]);

  readonly roleSearchQuery = signal('');
  readonly permissionSearchQuery = signal('');

  private readonly fallbackRoles: Role[] = [
    { name: 'admin', description: 'Administrator access profile', module_id: 'icarus-auth-ms', permissions: ['login', 'read', 'write'] },
    { name: 'standard-user', description: 'Standard user access profile', module_id: 'icarus-auth-ms', permissions: ['login', 'read'] }
  ];

  private readonly fallbackPermissions: Permission[] = [
    { id: 'login', module_id: 'icarus-auth-ms', action: 'login', description: 'Authenticate to systems' },
    { id: 'read', module_id: 'icarus-auth-ms', action: 'read', description: 'Read system state' },
    { id: 'write', module_id: 'icarus-auth-ms', action: 'write', description: 'Modify system configuration' }
  ];

  readonly displayRoles = computed(() => {
    const list = this.roles();
    if (list.length > 0) return list;
    const ownRoles = this.userProfile()?.roles || [];
    const mergedRoles = [...this.fallbackRoles];
    ownRoles.forEach(rn => {
      if (!mergedRoles.some(r => r.name === rn)) {
        mergedRoles.push({
          name: rn,
          module_id: rn.includes(' owner') ? rn.split(' ')[0] : 'icarus-auth-ms',
          description: 'User assigned role profile',
          permissions: []
        });
      }
    });
    return mergedRoles;
  });

  readonly displayPermissions = computed(() => {
    const list = this.permissions();
    if (list.length > 0) return list;
    const ownPerms = this.authService.currentUser()?.permissions || [];
    const mergedPerms = [...this.fallbackPermissions];
    ownPerms.forEach(p => {
      const parts = p.split(':');
      const moduleId = parts.length > 1 ? parts[0] : 'icarus-auth-ms';
      const action = parts.length > 1 ? parts[1] : p;
      if (!mergedPerms.some(perm => perm.action === action && perm.module_id === moduleId)) {
        mergedPerms.push({
          id: p,
          module_id: moduleId,
          action: action,
          description: 'Granted permission action'
        });
      }
    });
    return mergedPerms;
  });

  readonly myActiveRoles = computed(() => {
    const currentRoleNames = this.userProfile()?.roles || [];
    const allRoles = this.displayRoles();
    return allRoles.filter(r => currentRoleNames.includes(r.name));
  });

  readonly myResolvedPermissions = computed(() => {
    const currentRoleNames = this.userProfile()?.roles || [];
    const allRoles = this.displayRoles();
    const resolvedPermissionsSet = new Set<string>();

    const jwtPerms = this.authService.currentUser()?.permissions || [];
    if (this.roles().length === 0 && jwtPerms.length > 0) {
      return jwtPerms;
    }

    const visitedRoles = new Set<string>();
    const traverse = (roleName: string) => {
      if (visitedRoles.has(roleName)) return;
      visitedRoles.add(roleName);

      const roleObj = allRoles.find(r => r.name === roleName);
      if (!roleObj) return;

      if (roleObj.permissions) {
        roleObj.permissions.forEach(p => resolvedPermissionsSet.add(p));
      }

      if (roleObj.nested_roles) {
        roleObj.nested_roles.forEach(nr => traverse(nr));
      }
    };

    currentRoleNames.forEach(rn => traverse(rn));
    return Array.from(resolvedPermissionsSet);
  });

  readonly myResolvedPermissionDetails = computed(() => {
    const actions = this.myResolvedPermissions();
    const allPerms = this.displayPermissions();
    return allPerms.filter(p => actions.includes(p.module_id + ':' + p.action) || actions.includes(p.action));
  });

  readonly filteredActiveRoles = computed(() => {
    const query = this.roleSearchQuery().toLowerCase().trim();
    const list = this.myActiveRoles();
    if (!query) return list;
    return list.filter(r =>
      r.name.toLowerCase().includes(query) ||
      (r.module_id && r.module_id.toLowerCase().includes(query)) ||
      (r.description && r.description.toLowerCase().includes(query))
    );
  });

  readonly filteredResolvedPermissionDetails = computed(() => {
    const query = this.permissionSearchQuery().toLowerCase().trim();
    const list = this.myResolvedPermissionDetails();
    if (!query) return list;
    return list.filter(p =>
      p.action.toLowerCase().includes(query) ||
      (p.module_id && p.module_id.toLowerCase().includes(query)) ||
      (p.description && p.description.toLowerCase().includes(query))
    );
  });

  ngOnInit(): void {
    this.loadUserProfile();
    this.loadSystemMetadata();
  }

  loadUserProfile(): void {
    this.isLoadingProfile.set(true);
    this.profileError.set(null);
    this.authService.getProfile().subscribe({
      next: (data) => {
        this.userProfile.set(data);
        this.isLoadingProfile.set(false);
      },
      error: (err) => {
        this.profileError.set('Failed to load profile details.');
        this.isLoadingProfile.set(false);
      }
    });
  }

  loadSystemMetadata(): void {
    const isAdmin = this.authService.isAdmin();
    if (isAdmin) {
      this.adminService.listRoles().subscribe({
        next: (data) => this.roles.set(data as Role[]),
        error: (err) => console.warn('Failed to load system roles:', err)
      });
      this.adminService.listPermissions().subscribe({
        next: (data) => this.permissions.set(data as Permission[]),
        error: (err) => console.warn('Failed to load system permissions:', err)
      });
    } else {
      this.authService.getMyRoles().subscribe({
        next: (data) => this.roles.set(data as Role[]),
        error: (err) => console.warn('Failed to load user roles:', err)
      });
      this.authService.getMyPermissions().subscribe({
        next: (data) => this.permissions.set(data as Permission[]),
        error: (err) => console.warn('Failed to load user permissions:', err)
      });
    }
  }
}
