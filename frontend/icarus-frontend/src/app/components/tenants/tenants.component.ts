import { Component, signal, computed, inject, OnInit, viewChild, TemplateRef } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { PlatformService } from '../../services/platform.service';
import { forkJoin } from 'rxjs';

// Talos UI
import { TalosDataGridComponent, type TalosGridColDef } from '@weeehaoooz/talos-ui/data-display/data-grid';
import { TalosStatusTagComponent } from '@weeehaoooz/talos-ui/data-display/status-tag';
import { TalosFormFieldComponent } from '@weeehaoooz/talos-ui/form/form-field';
import { TalosPrefixDirective, TalosSuffixDirective } from '@weeehaoooz/talos-ui/form/affix';
import { TalosInputDirective } from '@weeehaoooz/talos-ui/form/input';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';
import { SelectInputComponent, OptionComponent } from '@weeehaoooz/talos-ui/form/select-input';

// Lucide Icons
import { LucideSearch, LucideX, LucidePlus, LucideTrash2 } from '@lucide/angular';

interface Tenant {
  id: string;
  code: string;
  name: string;
  status: string;
  created_at: string;
}

@Component({
  selector: 'app-tenants',
  imports: [
    FormsModule,
    DatePipe,
    TalosDataGridComponent,
    TalosFormFieldComponent,
    TalosPrefixDirective,
    TalosSuffixDirective,
    TalosInputDirective,
    TalosButtonDirective,
    TalosStatusTagComponent,
    TalosAlertComponent,
    SelectInputComponent,
    OptionComponent,
    LucideSearch,
    LucideX,
    LucidePlus,
    LucideTrash2
  ],
  templateUrl: './tenants.component.html',
  styleUrl: './tenants.component.scss'
})
export class TenantsComponent implements OnInit {
  private readonly platformService = inject(PlatformService);

  // Template Refs
  readonly selectTmpl = viewChild<TemplateRef<any>>('selectTmpl');
  readonly codeTmpl = viewChild<TemplateRef<any>>('codeTmpl');
  readonly statusTmpl = viewChild<TemplateRef<any>>('statusTmpl');
  readonly dateTmpl = viewChild<TemplateRef<any>>('dateTmpl');
  readonly actionsTmpl = viewChild<TemplateRef<any>>('actionsTmpl');

  // Data Signals
  readonly tenants = signal<Tenant[]>([]);
  readonly searchQuery = signal('');
  readonly isSearching = signal(false);
  readonly isLoading = signal(false);
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

  // Columns definition for talos-data-grid
  readonly columns = computed<TalosGridColDef<Tenant>[]>(() => [
    { field: 'select', header: '', width: '48px', minWidth: '48px', sortable: false, filterable: false, cellTemplate: this.selectTmpl() },
    { field: 'name', header: 'Name', sortType: 'string', filterMode: 'set', minWidth: '180px' },
    { field: 'code', header: 'Tenant Code', sortType: 'string', filterMode: 'set', minWidth: '150px', cellTemplate: this.codeTmpl() },
    { field: 'status', header: 'Status', minWidth: '120px', width: '130px', cellTemplate: this.statusTmpl() },
    { field: 'created_at', header: 'Onboarded At', sortType: 'date', minWidth: '160px', cellTemplate: this.dateTmpl() },
    { field: 'actions', header: 'Actions', width: '100px', minWidth: '100px', align: 'right', sortable: false, filterable: false, cellTemplate: this.actionsTmpl() }
  ]);

  // Is everything selected
  readonly isAllSelected = computed(() => {
    const list = this.filteredTenants();
    if (list.length === 0) return false;
    const selected = this.selectedIds();
    return list.every(t => selected.has(t.id));
  });

  onRowClick(event: { row: Tenant; index: number }): void {
    this.openEditPanel(event.row);
  }

  ngOnInit(): void {
    this.loadData();
  }

  loadData(): void {
    this.isLoading.set(true);
    this.platformService.listTenants().subscribe({
      next: (data) => {
        this.tenants.set((data as Tenant[]) || []);
        this.isLoading.set(false);
      },
      error: (err) => {
        console.error('Failed to load tenants:', err);
        this.isLoading.set(false);
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
