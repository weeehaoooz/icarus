import { Component, signal, computed, inject, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { PlatformService } from '../../services/platform.service';
import { forkJoin } from 'rxjs';

interface Application {
  id: string;
  code: string;
  name: string;
  description: string;
  created_at: string;
}

@Component({
  selector: 'app-applications',
  imports: [FormsModule, DatePipe],
  templateUrl: './applications.component.html',
  styleUrl: './applications.component.scss'
})
export class ApplicationsComponent implements OnInit {
  private readonly platformService = inject(PlatformService);

  // Data Signals
  readonly applications = signal<Application[]>([]);
  readonly searchQuery = signal('');
  readonly isSearching = signal(false);
  private searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Selection & Bulk Signals
  readonly selectedIds = signal<Set<string>>(new Set());

  // Panel Signals
  readonly activePanel = signal<'create' | 'edit' | null>(null);
  readonly panelError = signal<string | null>(null);
  readonly selectedApplication = signal<Application | null>(null);

  // Form State
  formData = {
    id: '',
    code: '',
    name: '',
    description: ''
  };

  // Filtered list
  readonly filteredApplications = computed(() => {
    const query = this.searchQuery().toLowerCase().trim();
    const allApps = this.applications() || [];
    if (!query) return allApps;
    return allApps.filter(a => 
      a.name.toLowerCase().includes(query) ||
      a.code.toLowerCase().includes(query) ||
      (a.description && a.description.toLowerCase().includes(query))
    );
  });

  // Is everything selected
  readonly isAllSelected = computed(() => {
    const list = this.filteredApplications();
    if (list.length === 0) return false;
    const selected = this.selectedIds();
    return list.every(a => selected.has(a.id));
  });

  ngOnInit(): void {
    this.loadData();
  }

  loadData(): void {
    this.platformService.listApplications().subscribe({
      next: (data) => this.applications.set((data as Application[]) || []),
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
    const list = this.filteredApplications();

    if (checked) {
      list.forEach(a => current.add(a.id));
    } else {
      list.forEach(a => current.delete(a.id));
    }
    this.selectedIds.set(current);
  }

  clearSelection(): void {
    this.selectedIds.set(new Set());
  }

  // Edit / Details panel logic
  openCreatePanel(): void {
    this.selectedApplication.set(null);
    this.panelError.set(null);
    this.formData = {
      id: '',
      code: '',
      name: '',
      description: ''
    };
    this.activePanel.set('create');
  }

  openEditPanel(app: Application): void {
    this.selectedApplication.set(app);
    this.panelError.set(null);
    this.formData = {
      id: app.id,
      code: app.code,
      name: app.name,
      description: app.description || ''
    };
    this.activePanel.set('edit');
  }

  closePanel(): void {
    this.activePanel.set(null);
    this.selectedApplication.set(null);
  }

  saveApplication(): void {
    this.panelError.set(null);

    const payload = {
      id: this.formData.id || this.formData.code,
      code: this.formData.code,
      name: this.formData.name,
      description: this.formData.description
    };

    if (this.activePanel() === 'edit') {
      this.platformService.updateApplication(this.formData.id, {
        code: this.formData.code,
        name: this.formData.name,
        description: this.formData.description
      }).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to update application.');
        }
      });
    } else {
      this.platformService.createApplication(payload).subscribe({
        next: () => {
          this.closePanel();
          this.loadData();
        },
        error: (err) => {
          this.panelError.set(err.error?.error || 'Failed to register application.');
        }
      });
    }
  }

  deleteApplication(id: string, event: Event): void {
    event.stopPropagation();
    if (confirm('Are you sure you want to delete this application? All associated roles and policies will be removed.')) {
      this.platformService.deleteApplication(id).subscribe({
        next: () => {
          const current = new Set(this.selectedIds());
          current.delete(id);
          this.selectedIds.set(current);
          this.loadData();
          if (this.selectedApplication()?.id === id) {
            this.closePanel();
          }
        },
        error: (err) => console.error('Failed to delete application:', err)
      });
    }
  }

  deleteSelected(): void {
    const ids = Array.from(this.selectedIds());
    if (ids.length === 0) return;

    if (confirm(`Are you sure you want to delete the ${ids.length} selected applications?`)) {
      const requests = ids.map(id => this.platformService.deleteApplication(id));
      forkJoin(requests).subscribe({
        next: () => {
          this.selectedIds.set(new Set());
          this.loadData();
          this.closePanel();
        },
        error: (err) => console.error('Failed to delete selected applications:', err)
      });
    }
  }
}
