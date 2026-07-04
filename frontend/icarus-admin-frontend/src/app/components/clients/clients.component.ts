import { Component, signal, computed, inject, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { AdminService } from '../../services/admin.service';
import { forkJoin, of } from 'rxjs';

interface Client {
  client_id: string;
  public_key: string;
  created_at: string;
  roles: string[];
}

interface Role {
  name: string;
  description: string;
}

@Component({
  selector: 'app-clients',
  imports: [FormsModule, DatePipe],
  templateUrl: './clients.component.html',
  styleUrl: './clients.component.scss'
})
export class ClientsComponent implements OnInit {
  private readonly adminService = inject(AdminService);

  // Data Signals
  readonly clients = signal<Client[]>([]);
  readonly availableRoles = signal<Role[]>([]);
  readonly searchQuery = signal('');
  readonly isSearching = signal(false);
  private searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Selection & Bulk Signals
  readonly selectedIds = signal<Set<string>>(new Set());

  // Panel Signals (replaces modal)
  readonly activePanel = signal<'create' | 'edit' | null>(null);
  readonly panelError = signal<string | null>(null);
  readonly selectedClient = signal<Client | null>(null);

  // Form State
  formData = {
    clientId: '',
    publicKey: '',
    roles: [] as string[]
  };

  // Filtered list
  readonly filteredClients = computed(() => {
    const query = this.searchQuery().toLowerCase().trim();
    const allClients = this.clients();
    if (!query) return allClients;
    return allClients.filter(c => 
      c.client_id.toLowerCase().includes(query)
    );
  });

  // Is everything selected
  readonly isAllSelected = computed(() => {
    const list = this.filteredClients();
    if (list.length === 0) return false;
    const selected = this.selectedIds();
    return list.every(c => selected.has(c.client_id));
  });

  ngOnInit(): void {
    this.loadData();
  }

  loadData(): void {
    this.adminService.listClients().subscribe({
      next: (data) => this.clients.set((data as Client[]) || []),
      error: (err) => {
        console.error('Failed to load clients:', err);
        this.clients.set([]);
      }
    });

    this.adminService.listRoles().subscribe({
      next: (data) => this.availableRoles.set((data as Role[]) || []),
      error: (err) => {
        console.error('Failed to load roles:', err);
        this.availableRoles.set([]);
      }
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
  toggleSelect(clientId: string, event: Event): void {
    event.stopPropagation();
    const current = new Set(this.selectedIds());
    if (current.has(clientId)) {
      current.delete(clientId);
    } else {
      current.add(clientId);
    }
    this.selectedIds.set(current);
  }

  toggleSelectAll(event: Event): void {
    const checked = (event.target as HTMLInputElement).checked;
    const current = new Set(this.selectedIds());
    const list = this.filteredClients();

    if (checked) {
      list.forEach(c => current.add(c.client_id));
    } else {
      list.forEach(c => current.delete(c.client_id));
    }
    this.selectedIds.set(current);
  }

  clearSelection(): void {
    this.selectedIds.set(new Set());
  }

  // Edit / Details panel logic
  openCreatePanel(): void {
    this.selectedClient.set(null);
    this.panelError.set(null);
    this.formData = {
      clientId: '',
      publicKey: '',
      roles: []
    };
    this.activePanel.set('create');
  }

  openEditPanel(client: Client): void {
    this.selectedClient.set(client);
    this.panelError.set(null);
    this.formData = {
      clientId: client.client_id,
      publicKey: client.public_key,
      roles: [...client.roles]
    };
    this.activePanel.set('edit');
  }

  closePanel(): void {
    this.activePanel.set(null);
    this.selectedClient.set(null);
  }

  toggleRole(roleName: string): void {
    const currentRoles = this.formData.roles;
    if (currentRoles.includes(roleName)) {
      this.formData.roles = currentRoles.filter(r => r !== roleName);
    } else {
      this.formData.roles = [...currentRoles, roleName];
    }
  }

  isRoleSelected(roleName: string): boolean {
    return this.formData.roles.includes(roleName);
  }

  saveClient(): void {
    this.panelError.set(null);

    const payload = {
      client_id: this.formData.clientId,
      public_key: this.formData.publicKey,
      roles: this.formData.roles
    };

    if (this.activePanel() === 'edit') {
      this.adminService.updateClient(this.formData.clientId, {
        public_key: this.formData.publicKey,
        roles: this.formData.roles
      }).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to update client.');
        }
      });
    } else {
      this.adminService.createClient(payload).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to register client.');
        }
      });
    }
  }

  deleteClient(clientId: string, event: Event): void {
    event.stopPropagation();
    if (confirm(`Are you sure you want to delete client '${clientId}'?`)) {
      this.adminService.deleteClient(clientId).subscribe({
        next: () => {
          if (this.selectedClient()?.client_id === clientId) {
            this.closePanel();
          }
          const selected = new Set(this.selectedIds());
          selected.delete(clientId);
          this.selectedIds.set(selected);
          this.loadData();
        },
        error: (err) => alert(err.error?.error || 'Failed to delete client.')
      });
    }
  }

  // Bulk Actions
  bulkDelete(): void {
    const ids = Array.from(this.selectedIds());
    if (ids.length === 0) return;
    if (confirm(`Are you sure you want to delete ${ids.length} selected clients?`)) {
      const requests = ids.map(id => this.adminService.deleteClient(id));
      forkJoin(requests).subscribe({
        next: () => {
          this.selectedIds.set(new Set());
          this.closePanel();
          this.loadData();
        },
        error: (err) => alert('Failed to delete some selected clients.')
      });
    }
  }

  bulkAssignRole(roleName: string): void {
    const ids = Array.from(this.selectedIds());
    if (ids.length === 0 || !roleName) return;
    if (confirm(`Assign role '${roleName}' to ${ids.length} selected clients?`)) {
      const requests = ids.map(id => {
        const c = this.clients().find(client => client.client_id === id);
        if (!c) return of(null);
        const mergedRoles = Array.from(new Set([...c.roles, roleName]));
        return this.adminService.updateClient(id, {
          public_key: c.public_key,
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
