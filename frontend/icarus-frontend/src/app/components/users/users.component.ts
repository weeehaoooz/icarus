import { Component, signal, computed, inject, OnInit, ChangeDetectorRef, viewChild, TemplateRef } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AdminService } from '../../services/admin.service';
import { forkJoin, of } from 'rxjs';
import { Dialog } from '@angular/cdk/dialog';
import { RoleAssignmentDialogComponent } from './role-assignment-dialog/role-assignment-dialog.component';

// Talos UI
import { TalosDataGridComponent, type TalosGridColDef } from '@weeehaoooz/talos-ui/data-display/data-grid';
import { TalosStatusTagComponent } from '@weeehaoooz/talos-ui/data-display/status-tag';
import { TalosFormFieldComponent } from '@weeehaoooz/talos-ui/form/form-field';
import { TalosPrefixDirective, TalosSuffixDirective } from '@weeehaoooz/talos-ui/form/affix';
import { TalosInputDirective } from '@weeehaoooz/talos-ui/form/input';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';

// Lucide Icons
import { LucideSearch, LucideX, LucidePlus, LucideLock, LucideUnlock, LucideTrash2 } from '@lucide/angular';

interface User {
  id: number;
  username: string;
  first_name: string;
  last_name: string;
  email: string;
  is_active?: boolean;
  roles?: string[];
}

interface Role {
  name: string;
  description: string;
}

@Component({
  selector: 'app-users',
  imports: [
    FormsModule,
    TalosDataGridComponent,
    TalosFormFieldComponent,
    TalosPrefixDirective,
    TalosSuffixDirective,
    TalosInputDirective,
    TalosButtonDirective,
    TalosStatusTagComponent,
    LucideSearch,
    LucideX,
    LucidePlus,
    LucideLock,
    LucideUnlock,
    LucideTrash2
  ],
  templateUrl: './users.component.html',
  styleUrl: './users.component.scss'
})
export class UsersComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly dialog = inject(Dialog);
  private readonly cdr = inject(ChangeDetectorRef);

  // Template Refs
  readonly selectTmpl = viewChild<TemplateRef<any>>('selectTmpl');
  readonly usernameTmpl = viewChild<TemplateRef<any>>('usernameTmpl');
  readonly statusTmpl = viewChild<TemplateRef<any>>('statusTmpl');
  readonly rolesTmpl = viewChild<TemplateRef<any>>('rolesTmpl');
  readonly actionsTmpl = viewChild<TemplateRef<any>>('actionsTmpl');

  // Data Signals
  readonly users = signal<User[]>([]);
  readonly availableRoles = signal<Role[]>([]);
  readonly searchQuery = signal('');
  readonly isSearching = signal(false);
  readonly isLoading = signal(false);
  private searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Selection & Bulk Signals
  readonly selectedIds = signal<Set<number>>(new Set());

  // Panel Signals (replaces modal)
  readonly activePanel = signal<'create' | 'edit' | null>(null);
  readonly panelError = signal<string | null>(null);
  readonly selectedUser = signal<User | null>(null);

  // Form State
  formData = {
    id: 0,
    username: '',
    email: '',
    firstName: '',
    lastName: '',
    password: '',
    isActive: true,
    roles: [] as string[]
  };

  // Derived state: Filtered Users list
  readonly filteredUsers = computed(() => {
    const query = this.searchQuery().toLowerCase().trim();
    const allUsers = this.users();
    if (!query) return allUsers;
    return allUsers.filter(u =>
      u.username.toLowerCase().includes(query) ||
      u.first_name.toLowerCase().includes(query) ||
      u.last_name.toLowerCase().includes(query) ||
      (u.email && u.email.toLowerCase().includes(query))
    );
  });

  // Columns definition for talos-data-grid
  readonly columns = computed<TalosGridColDef<User>[]>(() => [
    { field: 'select', header: '', width: '48px', minWidth: '48px', sortable: false, filterable: false, cellTemplate: this.selectTmpl() },
    { field: 'id', header: 'ID', sortType: 'number', filterMode: 'condition', width: '80px', minWidth: '70px' },
    { field: 'username', header: 'Username', sortType: 'string', filterMode: 'set', minWidth: '160px', cellTemplate: this.usernameTmpl() },
    { field: 'first_name', header: 'Full Name', sortType: 'string', filterMode: 'set', minWidth: '160px', formatter: (_: any, u: User) => `${u.first_name || ''} ${u.last_name || ''}`.trim() || '-' },
    { field: 'email', header: 'Email', sortType: 'string', filterMode: 'set', minWidth: '200px', formatter: (v: any) => v || '-' },
    { field: 'is_active', header: 'Status', minWidth: '120px', width: '130px', cellTemplate: this.statusTmpl() },
    { field: 'roles', header: 'Roles', minWidth: '220px', cellTemplate: this.rolesTmpl() },
    { field: 'actions', header: 'Actions', width: '110px', minWidth: '110px', align: 'right', sortable: false, filterable: false, cellTemplate: this.actionsTmpl() }
  ]);

  // Derived state: Is everything in filtered list checked
  readonly isAllSelected = computed(() => {
    const list = this.filteredUsers();
    if (list.length === 0) return false;
    const selected = this.selectedIds();
    return list.every(u => selected.has(u.id));
  });

  ngOnInit(): void {
    this.loadData();
  }

  loadData(): void {
    this.isLoading.set(true);
    this.adminService.listUsers().subscribe({
      next: (data) => {
        const mappedUsers = (data as User[]).map(u => ({
          ...u,
          roles: u.roles || []
        }));
        this.users.set(mappedUsers);
        this.isLoading.set(false);
      },
      error: (err) => {
        console.error('Failed to load users:', err);
        this.isLoading.set(false);
      }
    });

    this.adminService.listRoles().subscribe({
      next: (data) => this.availableRoles.set(data as Role[]),
      error: (err) => console.error('Failed to load roles:', err)
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

  // Row Selection logic
  toggleSelect(id: number, event: Event): void {
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
    const list = this.filteredUsers();

    if (checked) {
      list.forEach(u => current.add(u.id));
    } else {
      list.forEach(u => current.delete(u.id));
    }
    this.selectedIds.set(current);
  }

  clearSelection(): void {
    this.selectedIds.set(new Set());
  }

  onRowClick(event: { row: User; index: number }): void {
    this.openEditPanel(event.row);
  }

  // Edit / Details panel logic
  openCreatePanel(): void {
    this.selectedUser.set(null);
    this.panelError.set(null);
    this.formData = {
      id: 0,
      username: '',
      email: '',
      firstName: '',
      lastName: '',
      password: '',
      isActive: true,
      roles: []
    };
    this.activePanel.set('create');
  }

  openEditPanel(user: User): void {
    this.selectedUser.set(user);
    this.panelError.set(null);
    this.formData = {
      id: user.id,
      username: user.username,
      email: user.email,
      firstName: user.first_name,
      lastName: user.last_name,
      password: '',
      isActive: user.is_active !== false,
      roles: user.roles ? [...user.roles] : []
    };
    this.activePanel.set('edit');
  }

  closePanel(): void {
    this.activePanel.set(null);
    this.selectedUser.set(null);
  }

  toggleUserStatus(user: User, event: Event): void {
    event.stopPropagation();
    const newStatus = !user.is_active;
    const actionText = newStatus ? 'enable' : 'disable';
    if (confirm(`Are you sure you want to ${actionText} account '${user.username}'?`)) {
      this.adminService.updateUserStatus(user.id, newStatus).subscribe({
        next: () => {
          this.loadData();
          if (this.selectedUser()?.id === user.id) {
            this.formData.isActive = newStatus;
          }
        },
        error: (err) => alert(err.error?.error || `Failed to ${actionText} account.`)
      });
    }
  }

  openRoleModal(): void {
    const dialogRef = this.dialog.open<string[]>(RoleAssignmentDialogComponent, {
      width: '900px',
      maxWidth: '95vw',
      height: '600px',
      maxHeight: '85vh',
      data: {
        username: this.formData.username,
        email: this.formData.email,
        assignedRoles: [...this.formData.roles],
        availableRoles: this.availableRoles()
      }
    });

    dialogRef.closed.subscribe(result => {
      if (result !== undefined) {
        this.formData.roles = result;
        this.cdr.markForCheck();
      }
    });
  }

  saveUser(): void {
    this.panelError.set(null);

    const payload = {
      username: this.formData.username,
      email: this.formData.email,
      first_name: this.formData.firstName,
      last_name: this.formData.lastName,
      password: this.formData.password,
      is_active: this.formData.isActive,
      roles: this.formData.roles
    };

    if (this.activePanel() === 'edit') {
      this.adminService.updateUser(this.formData.id, payload).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to update user.');
        }
      });
    } else {
      if (!payload.password) {
        this.panelError.set('Password is required for new users.');
        return;
      }
      this.adminService.createUser(payload).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to create user.');
        }
      });
    }
  }

  deleteUser(id: number, event: Event): void {
    event.stopPropagation();
    if (confirm('Are you sure you want to delete this user?')) {
      this.adminService.deleteUser(id).subscribe({
        next: () => {
          if (this.selectedUser()?.id === id) {
            this.closePanel();
          }
          const selected = new Set(this.selectedIds());
          selected.delete(id);
          this.selectedIds.set(selected);
          this.loadData();
        },
        error: (err) => alert(err.error?.error || 'Failed to delete user.')
      });
    }
  }

  // Bulk Actions
  bulkDelete(): void {
    const ids = Array.from(this.selectedIds());
    if (ids.length === 0) return;
    if (confirm(`Are you sure you want to delete ${ids.length} selected users?`)) {
      const requests = ids.map(id => this.adminService.deleteUser(id));
      forkJoin(requests).subscribe({
        next: () => {
          this.selectedIds.set(new Set());
          this.closePanel();
          this.loadData();
        },
        error: (err) => alert('Failed to complete bulk deletion. Some accounts may not have been deleted.')
      });
    }
  }

  bulkUpdateStatus(isActive: boolean): void {
    const ids = Array.from(this.selectedIds());
    if (ids.length === 0) return;
    const actionText = isActive ? 'enable' : 'disable';
    if (confirm(`Are you sure you want to ${actionText} ${ids.length} selected accounts?`)) {
      const requests = ids.map(id => this.adminService.updateUserStatus(id, isActive));
      forkJoin(requests).subscribe({
        next: () => {
          this.selectedIds.set(new Set());
          this.loadData();
        },
        error: () => alert(`Failed to complete bulk ${actionText} operation.`)
      });
    }
  }

  bulkAssignRole(roleName: string): void {
    const ids = Array.from(this.selectedIds());
    if (ids.length === 0 || !roleName) return;
    if (confirm(`Assign the role '${roleName}' to ${ids.length} selected users?`)) {
      const requests = ids.map(id => {
        const u = this.users().find(user => user.id === id);
        if (!u) return of(null);

        const mergedRoles = Array.from(new Set([...(u.roles || []), roleName]));
        return this.adminService.updateUser(id, {
          username: u.username,
          email: u.email,
          first_name: u.first_name,
          last_name: u.last_name,
          is_active: u.is_active !== false,
          roles: mergedRoles
        });
      });

      forkJoin(requests).subscribe({
        next: () => {
          this.selectedIds.set(new Set());
          this.loadData();
        },
        error: (err) => alert('Failed to complete bulk role assignments.')
      });
    }
  }
}


