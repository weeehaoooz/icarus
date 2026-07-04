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

interface AccessRequest {
  id: string;
  tenant: string;
  module: string;
  role: string;
  justification: string;
  status: 'Pending' | 'Approved' | 'Rejected';
  submittedAt: string;
}

@Component({
  selector: 'app-request-access',
  imports: [FormsModule],
  templateUrl: './request-access.component.html',
  styleUrl: './request-access.component.scss'
})
export class RequestAccessComponent implements OnInit {
  private readonly authService = inject(AuthService);
  private readonly adminService = inject(AdminService);
  private readonly platformService = inject(PlatformService);

  readonly userProfile = signal<UserProfile | null>(null);
  readonly isLoadingProfile = signal(false);
  readonly profileError = signal<string | null>(null);

  readonly roles = signal<Role[]>([]);
  readonly modules = signal<any[]>([]);
  readonly tenants = signal<any[]>([]);

  requestForm = {
    tenant: 'system-tenant',
    module: 'icarus-auth-ms',
    role: '',
    justification: ''
  };
  readonly requestSuccessMessage = signal<string | null>(null);
  readonly requestErrorMessage = signal<string | null>(null);
  readonly requestHistory = signal<AccessRequest[]>([]);

  private readonly fallbackTenants = [
    { id: 'system-tenant', code: 'system-tenant', name: 'System Tenant' },
    { id: 'global', code: 'global', name: 'Global Scope' }
  ];

  private readonly fallbackModules = [
    { id: 'icarus-auth-ms', code: 'icarus-auth-ms', name: 'Auth Microservice' },
    { id: 'icarus-admin-ms', code: 'icarus-admin-ms', name: 'Admin Microservice' }
  ];

  private readonly fallbackRoles: Role[] = [
    { name: 'admin', description: 'Administrator access profile', module_id: 'icarus-auth-ms', permissions: ['login', 'read', 'write'] },
    { name: 'standard-user', description: 'Standard user access profile', module_id: 'icarus-auth-ms', permissions: ['login', 'read'] }
  ];

  readonly displayTenants = computed(() => {
    const list = this.tenants();
    if (list.length > 0) return list;
    const groups = this.userProfile()?.groups || [];
    const groupTenants = groups.map(g => ({ id: g.name, code: g.name, name: g.name }));
    return groupTenants.length > 0 ? groupTenants : this.fallbackTenants;
  });

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

  readonly requestableRoles = computed(() => {
    const currentRoles = this.userProfile()?.roles || [];
    return this.displayRoles().filter(r => !currentRoles.includes(r.name));
  });

  ngOnInit(): void {
    this.loadUserProfile();
    this.loadRequestHistory();
    this.loadRequestMetadata();
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

  loadRequestMetadata(): void {
    const isAdmin = this.authService.isAdmin();
    const isModuleOwner = this.authService.isModuleOwner();
    const hasElevatedPrivilege = isAdmin || isModuleOwner;

    if (!hasElevatedPrivilege) {
      return;
    }

    if (isAdmin) {
      this.adminService.listRoles().subscribe({
        next: (data) => this.roles.set(data as Role[]),
        error: (err) => console.warn('Failed to load system roles:', err)
      });
    }

    this.platformService.listModules().subscribe({
      next: (data) => this.modules.set(data),
      error: (err) => console.warn('Failed to load system modules:', err)
    });

    this.platformService.listTenants().subscribe({
      next: (data) => this.tenants.set(data),
      error: (err) => console.warn('Failed to load system tenants:', err)
    });
  }

  loadRequestHistory(): void {
    const stored = localStorage.getItem('access_requests');
    if (stored) {
      try {
        this.requestHistory.set(JSON.parse(stored));
      } catch (e) {
        this.requestHistory.set([]);
      }
    }
  }

  submitAccessRequest(): void {
    this.requestSuccessMessage.set(null);
    this.requestErrorMessage.set(null);

    if (!this.requestForm.role) {
      this.requestErrorMessage.set('Please select a role to request.');
      return;
    }
    if (this.requestForm.justification.trim().length < 10) {
      this.requestErrorMessage.set('Justification must be at least 10 characters.');
      return;
    }

    const isAdminUser = this.authService.isAdmin();
    const status = isAdminUser ? 'Approved' : 'Pending';

    const newRequest: AccessRequest = {
      id: 'REQ-' + Math.floor(100000 + Math.random() * 900000),
      tenant: this.requestForm.tenant,
      module: this.requestForm.module,
      role: this.requestForm.role,
      justification: this.requestForm.justification,
      status: status,
      submittedAt: new Date().toLocaleString()
    };

    const history = [newRequest, ...this.requestHistory()];
    this.requestHistory.set(history);
    localStorage.setItem('access_requests', JSON.stringify(history));

    if (isAdminUser) {
      const profile = this.userProfile();
      if (profile) {
        const currentRoles = profile.roles || [];
        const updatedRoles = Array.from(new Set([...currentRoles, this.requestForm.role]));

        this.adminService.updateUser(profile.id, {
          username: profile.username,
          email: profile.email,
          first_name: profile.first_name,
          last_name: profile.last_name,
          password: '',
          roles: updatedRoles
        }).subscribe({
          next: () => {
            this.requestSuccessMessage.set('Access request submitted and auto-approved! Your roles have been updated.');
            this.loadUserProfile();
            this.requestForm.role = '';
            this.requestForm.justification = '';
          },
          error: (err) => {
            this.requestErrorMessage.set('Request logged, but database self-assignment failed: ' + (err.error?.error || err.message));
          }
        });
      }
    } else {
      this.requestSuccessMessage.set('Access request submitted successfully. It is pending administrator review.');
      this.requestForm.role = '';
      this.requestForm.justification = '';
    }
  }
}
