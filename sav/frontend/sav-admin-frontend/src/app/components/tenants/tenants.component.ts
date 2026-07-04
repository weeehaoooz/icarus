import { Component, signal, computed, inject, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { PlatformService } from '../../services/platform.service';
import { forkJoin } from 'rxjs';

interface Tenant {
  id: string;
  code: string;
  name: string;
  status: string;
  created_at: string;
}

@Component({
  selector: 'app-tenants',
  imports: [FormsModule, DatePipe],
  templateUrl: './tenants.component.html',
  styleUrl: './tenants.component.scss'
})
export class TenantsComponent implements OnInit {
  private readonly platformService = inject(PlatformService);

  // Data Signals
  readonly tenants = signal<Tenant[]>([]);
  readonly searchQuery = signal('');
  readonly isSearching = signal(false);
  private searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Selection & Bulk Signals
  readonly selectedIds = signal<Set<string>>(new Set());

  // Panel Signals
  readonly activePanel = signal<'create' | 'edit' | null>(null);
  readonly panelError = signal<string | null>(null);
  readonly selectedTenant = signal<Tenant | null>(null);

  // Form State
  formData = {
    id: '',
    code: '',
    name: '',
    status: 'active'
  };

  // Filtered list
  readonly filteredTenants = computed(() => {
    const query = this.searchQuery().toLowerCase().trim();
    const allTenants = this.tenants() || [];
    if (!query) return allTenants;
    return allTenants.filter(t => 
      t.name.toLowerCase().includes(query) ||
      t.code.toLowerCase().includes(query) ||
      t.status.toLowerCase().includes(query)
    );
  });

  // Is everything selected
  readonly isAllSelected = computed(() => {
    const list = this.filteredTenants();
    if (list.length === 0) return false;
    const selected = this.selectedIds();
    return list.every(t => selected.has(t.id));
  });

  ngOnInit(): void {
    this.loadData();
  }

  loadData(): void {
    this.platformService.listTenants().subscribe({
      next: (data) => this.tenants.set((data as Tenant[]) || []),
      error: (err) => console.error('Failed to load tenants:', err)
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
    const list = this.filteredTenants();

    if (checked) {
      list.forEach(t => current.add(t.id));
    } else {
      list.forEach(t => current.delete(t.id));
    }
    this.selectedIds.set(current);
  }

  clearSelection(): void {
    this.selectedIds.set(new Set());
  }

  // Edit / Details panel logic
  openCreatePanel(): void {
    this.selectedTenant.set(null);
    this.panelError.set(null);
    this.formData = {
      id: '',
      code: '',
      name: '',
      status: 'active'
    };
    this.activePanel.set('create');
  }

  openEditPanel(tenant: Tenant): void {
    this.selectedTenant.set(tenant);
    this.panelError.set(null);
    this.formData = {
      id: tenant.id,
      code: tenant.code,
      name: tenant.name,
      status: tenant.status
    };
    this.activePanel.set('edit');
  }

  closePanel(): void {
    this.activePanel.set(null);
    this.selectedTenant.set(null);
  }

  saveTenant(): void {
    this.panelError.set(null);

    const payload = {
      id: this.formData.id || this.formData.code,
      code: this.formData.code,
      name: this.formData.name,
      status: this.formData.status
    };

    if (this.activePanel() === 'edit') {
      this.platformService.updateTenant(this.formData.id, {
        code: this.formData.code,
        name: this.formData.name,
        status: this.formData.status
      }).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to update tenant.');
        }
      });
    } else {
      this.platformService.createTenant(payload).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to register tenant.');
        }
      });
    }
  }

  deleteTenant(id: string, event: Event): void {
    event.stopPropagation();
    if (confirm('Are you sure you want to delete this tenant? All associated roles and policies will be removed.')) {
      this.platformService.deleteTenant(id).subscribe({
        next: () => {
          const current = new Set(this.selectedIds());
          current.delete(id);
          this.selectedIds.set(current);
          this.loadData();
          if (this.selectedTenant()?.id === id) {
            this.closePanel();
          }
        },
        error: (err) => console.error('Failed to delete tenant:', err)
      });
    }
  }

  deleteSelected(): void {
    const ids = Array.from(this.selectedIds());
    if (ids.length === 0) return;

    if (confirm(`Are you sure you want to delete the ${ids.length} selected tenants?`)) {
      const requests = ids.map(id => this.platformService.deleteTenant(id));
      forkJoin(requests).subscribe({
        next: () => {
          this.selectedIds.set(new Set());
          this.loadData();
          this.closePanel();
        },
        error: (err) => console.error('Failed to delete selected tenants:', err)
      });
    }
  }
}
