import { Component, signal, computed, inject, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AuthService } from '../../services/auth.service';
import { AdminService } from '../../services/admin.service';
import { PlatformService } from '../../services/platform.service';

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
  selector: 'app-policy-sandbox',
  imports: [FormsModule],
  templateUrl: './policy-sandbox.component.html',
  styleUrl: './policy-sandbox.component.scss'
})
export class PolicySandboxComponent implements OnInit {
  private readonly authService = inject(AuthService);
  private readonly adminService = inject(AdminService);
  private readonly platformService = inject(PlatformService);

  readonly userProfile = signal<UserProfile | null>(null);
  readonly isLoadingProfile = signal(false);
  readonly profileError = signal<string | null>(null);

  readonly roles = signal<Role[]>([]);
  readonly permissions = signal<Permission[]>([]);
  readonly modules = signal<any[]>([]);

  sandboxForm = {
    module: 'icarus-auth-ms',
    action: ''
  };
  readonly isEvaluating = signal(false);
  readonly evaluationResult = signal<'GRANTED' | 'DENIED' | null>(null);
  readonly evaluationTrace = signal<string[]>([]);

  private readonly fallbackModules = [
    { id: 'icarus-auth-ms', code: 'icarus-auth-ms', name: 'Auth Microservice' },
    { id: 'icarus-admin-ms', code: 'icarus-admin-ms', name: 'Admin Microservice' }
  ];

  private readonly fallbackRoles: Role[] = [
    { name: 'admin', description: 'Administrator access profile', module_id: 'icarus-auth-ms', permissions: ['login', 'read', 'write'] },
    { name: 'standard-user', description: 'Standard user access profile', module_id: 'icarus-auth-ms', permissions: ['login', 'read'] }
  ];

  private readonly fallbackPermissions: Permission[] = [
    { id: 'login', module_id: 'icarus-auth-ms', action: 'login', description: 'Authenticate to systems' },
    { id: 'read', module_id: 'icarus-auth-ms', action: 'read', description: 'Read system state' },
    { id: 'write', module_id: 'icarus-auth-ms', action: 'write', description: 'Modify system configuration' }
  ];

  readonly displayModules = computed(() => {
    const list = this.modules();
    if (list.length > 0) return list;
    const owned = this.authService.currentUser()?.owned_modules || [];
    const ownedList = owned.map(m => ({ id: m, code: m, name: m }));
    return ownedList.length > 0 ? ownedList : this.fallbackModules;
  });

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

  readonly sandboxActions = computed(() => {
    const modId = this.sandboxForm.module;
    return this.displayPermissions()
      .filter(p => p.module_id === modId)
      .map(p => p.action);
  });

  ngOnInit(): void {
    this.loadUserProfile();
    this.loadSandboxData();
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

  loadSandboxData(): void {
    const isAdmin = this.authService.isAdmin();
    const isModuleOwner = this.authService.isModuleOwner();
    const hasElevatedPrivilege = isAdmin || isModuleOwner;

    if (!hasElevatedPrivilege) {
      // Standard users use fallbacks and profile data
      const actions = this.sandboxActions();
      if (actions.length > 0) {
        this.sandboxForm.action = actions[0];
      }
      return;
    }

    if (isAdmin) {
      this.adminService.listPermissions().subscribe({
        next: (data) => {
          this.permissions.set(data as Permission[]);
          const actions = this.sandboxActions();
          if (actions.length > 0) {
            this.sandboxForm.action = actions[0];
          }
        },
        error: (err) => console.warn('Failed to load system permissions:', err)
      });
      
      this.adminService.listRoles().subscribe({
        next: (data) => this.roles.set(data as Role[]),
        error: (err) => console.warn('Failed to load system roles:', err)
      });
    }

    this.platformService.listModules().subscribe({
      next: (data) => this.modules.set(data),
      error: (err) => console.warn('Failed to load system modules:', err)
    });
  }

  onSandboxModuleChange(): void {
    const actions = this.sandboxActions();
    this.sandboxForm.action = actions.length > 0 ? actions[0] : '';
  }

  simulatePolicyEvaluation(): void {
    this.isEvaluating.set(true);
    this.evaluationResult.set(null);
    this.evaluationTrace.set([]);

    const targetModule = this.sandboxForm.module;
    const targetAction = this.sandboxForm.action;
    const trace: string[] = [];

    trace.push(`[${new Date().toLocaleTimeString()}] Initializing access verification sandbox...`);
    trace.push(`Checking policies for user: '${this.userProfile()?.username}'`);

    setTimeout(() => {
      const activeRoles = this.userProfile()?.roles || [];
      trace.push(`Detected ${activeRoles.length} active role bindings: ${JSON.stringify(activeRoles)}`);

      const allRoles = this.displayRoles();
      const resolvedPermissionsSet = new Set<string>();
      const visitedRoles = new Set<string>();

      const traverse = (roleName: string, depth = 0) => {
        const indent = '  '.repeat(depth);
        if (visitedRoles.has(roleName)) {
          trace.push(`${indent}↳ Role '${roleName}' already visited. Skipping cycle.`);
          return;
        }
        visitedRoles.add(roleName);
        trace.push(`${indent}Evaluating Role: '${roleName}'`);

        const roleObj = allRoles.find(r => r.name === roleName);
        if (!roleObj) {
          trace.push(`${indent}↳ Role metadata not found in active module definitions.`);
          return;
        }

        if (roleObj.permissions && roleObj.permissions.length > 0) {
          trace.push(`${indent}  Direct permissions: ${JSON.stringify(roleObj.permissions)}`);
          roleObj.permissions.forEach(p => resolvedPermissionsSet.add(p));
        }

        if (roleObj.nested_roles && roleObj.nested_roles.length > 0) {
          trace.push(`${indent}  Nested roles detected: ${JSON.stringify(roleObj.nested_roles)}`);
          roleObj.nested_roles.forEach(nr => traverse(nr, depth + 1));
        }
      };

      activeRoles.forEach(rn => traverse(rn));

      const resolvedActions = Array.from(resolvedPermissionsSet);
      trace.push(`Fully resolved hierarchical permissions list: ${JSON.stringify(resolvedActions)}`);

      const exactMatch = targetModule + ':' + targetAction;
      trace.push(`Matching target action '${exactMatch}' or '${targetAction}' against resolved permission set...`);

      const isGranted = resolvedActions.some(act => act === exactMatch || act === targetAction);

      if (isGranted) {
        this.evaluationResult.set('GRANTED');
        trace.push(`\u2705 MATCH FOUND! Access policy evaluates to ALLOW.`);
      } else {
        this.evaluationResult.set('DENIED');
        trace.push(`\u274c NO MATCH FOUND! Access policy evaluates to DENIED.`);
      }

      this.evaluationTrace.set(trace);
      this.isEvaluating.set(false);
    }, 600);
  }
}
