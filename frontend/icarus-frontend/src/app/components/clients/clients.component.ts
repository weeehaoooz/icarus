import { Component, signal, computed, inject, OnInit, ChangeDetectorRef, viewChild, TemplateRef } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AdminService } from '../../services/admin.service';
import { forkJoin, of } from 'rxjs';
import { Dialog } from '@angular/cdk/dialog';
import { RoleAssignmentDialogComponent } from '../users/role-assignment-dialog/role-assignment-dialog.component';

// Talos UI
import { TalosDataGridComponent, type TalosGridColDef } from '@weeehaoooz/talos-ui/data-display/data-grid';
import { TalosFormFieldComponent } from '@weeehaoooz/talos-ui/form/form-field';
import { TalosPrefixDirective, TalosSuffixDirective } from '@weeehaoooz/talos-ui/form/affix';
import { TalosInputDirective } from '@weeehaoooz/talos-ui/form/input';
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';
import { TalosCheckboxDirective } from '@weeehaoooz/talos-ui/form/checkbox';
import { SelectInputComponent, OptionComponent } from '@weeehaoooz/talos-ui/form/select-input';

// Lucide Icons
import {
  LucideSearch,
  LucideX,
  LucidePlus,
  LucideTrash2,
  LucideKey,
  LucideDownload,
  LucideCopy,
  LucideCheck,
  LucideRefreshCw,
  LucideShield,
  LucideSlidersHorizontal,
  LucidePencil
} from '@lucide/angular';

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
  imports: [
    FormsModule,
    TalosDataGridComponent,
    TalosFormFieldComponent,
    TalosPrefixDirective,
    TalosSuffixDirective,
    TalosInputDirective,
    TalosButtonDirective,
    TalosAlertComponent,
    TalosCheckboxDirective,
    SelectInputComponent,
    OptionComponent,
    LucideSearch,
    LucideX,
    LucidePlus,
    LucideTrash2,
    LucideKey,
    LucideDownload,
    LucideCopy,
    LucideCheck,
    LucideRefreshCw,
    LucideShield,
    LucidePencil
  ],
  templateUrl: './clients.component.html',
  styleUrl: './clients.component.scss'
})
export class ClientsComponent implements OnInit {
  private readonly adminService = inject(AdminService);
  private readonly dialog = inject(Dialog);
  private readonly cdr = inject(ChangeDetectorRef);

  // Template Refs
  readonly selectTmpl = viewChild<TemplateRef<any>>('selectTmpl');
  readonly clientIdTmpl = viewChild<TemplateRef<any>>('clientIdTmpl');
  readonly rolesTmpl = viewChild<TemplateRef<any>>('rolesTmpl');
  readonly actionsTmpl = viewChild<TemplateRef<any>>('actionsTmpl');

  // Data Signals
  readonly clients = signal<Client[]>([]);
  readonly availableRoles = signal<Role[]>([]);
  readonly searchQuery = signal('');
  readonly isSearching = signal(false);
  readonly isLoading = signal(false);
  private searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;

  // Selection & Bulk Signals
  readonly selectedIds = signal<Set<string>>(new Set());

  // Panel Signals (replaces modal)
  readonly activePanel = signal<'create' | 'edit' | null>(null);
  readonly panelError = signal<string | null>(null);
  readonly selectedClient = signal<Client | null>(null);

  // Keypair Generation State
  readonly isGeneratingKeys = signal(false);
  readonly generatedPrivateKey = signal<string | null>(null);
  readonly keyGenSuccess = signal<string | null>(null);
  readonly keyCopied = signal(false);

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
      c.client_id.toLowerCase().includes(query) ||
      (c.roles || []).some(r => r.toLowerCase().includes(query))
    );
  });

  // Columns definition for talos-data-grid (No public key preview)
  readonly columns = computed<TalosGridColDef<Client>[]>(() => [
    { field: 'select', header: '', width: '48px', minWidth: '48px', sortable: false, filterable: false, cellTemplate: this.selectTmpl() },
    { field: 'client_id', header: 'Client ID', sortType: 'string', filterMode: 'set', minWidth: '220px', cellTemplate: this.clientIdTmpl() },
    { field: 'roles', header: 'Assigned Roles', minWidth: '260px', cellTemplate: this.rolesTmpl() },
    { field: 'created_at', header: 'Created At', sortType: 'date', filterMode: 'condition', minWidth: '180px', formatter: (v) => v ? new Date(v).toLocaleString() : '-' },
    { field: 'actions', header: 'Actions', width: '110px', minWidth: '110px', align: 'right', sortable: false, filterable: false, cellTemplate: this.actionsTmpl() }
  ]);

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
    this.isLoading.set(true);
    this.adminService.listClients().subscribe({
      next: (data) => {
        this.clients.set((data as Client[]) || []);
        this.isLoading.set(false);
      },
      error: (err) => {
        console.error('Failed to load clients:', err);
        this.clients.set([]);
        this.isLoading.set(false);
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

  onRowClick(event: { row: Client; index: number }): void {
    this.openEditPanel(event.row);
  }

  // Edit / Details panel logic
  openCreatePanel(): void {
    this.selectedClient.set(null);
    this.panelError.set(null);
    this.generatedPrivateKey.set(null);
    this.keyGenSuccess.set(null);
    this.keyCopied.set(false);
    this.formData = {
      clientId: '',
      publicKey: '',
      roles: []
    };
    this.activePanel.set('create');
  }

  openEditPanel(client: Client, event?: Event): void {
    event?.stopPropagation();
    this.selectedClient.set(client);
    this.panelError.set(null);
    this.generatedPrivateKey.set(null);
    this.keyGenSuccess.set(null);
    this.keyCopied.set(false);
    this.formData = {
      clientId: client.client_id,
      publicKey: client.public_key || '',
      roles: client.roles ? [...client.roles] : []
    };
    this.activePanel.set('edit');
  }

  closePanel(): void {
    this.activePanel.set(null);
    this.selectedClient.set(null);
    this.generatedPrivateKey.set(null);
    this.keyGenSuccess.set(null);
    this.keyCopied.set(false);
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

  openRoleModal(): void {
    const dialogRef = this.dialog.open<string[]>(RoleAssignmentDialogComponent, {
      width: '900px',
      maxWidth: '95vw',
      height: '600px',
      maxHeight: '85vh',
      data: {
        username: this.formData.clientId || 'New Client',
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

  openRoleModalForClient(client: Client, event?: Event): void {
    event?.stopPropagation();
    const dialogRef = this.dialog.open<string[]>(RoleAssignmentDialogComponent, {
      width: '900px',
      maxWidth: '95vw',
      height: '600px',
      maxHeight: '85vh',
      data: {
        username: client.client_id,
        assignedRoles: client.roles ? [...client.roles] : [],
        availableRoles: this.availableRoles()
      }
    });

    dialogRef.closed.subscribe(result => {
      if (result !== undefined) {
        this.adminService.updateClient(client.client_id, {
          public_key: client.public_key,
          roles: result
        }).subscribe({
          next: () => {
            this.loadData();
            if (this.selectedClient()?.client_id === client.client_id) {
              this.formData.roles = result;
            }
          },
          error: (err) => alert(err.error?.error || 'Failed to update client roles.')
        });
      }
    });
  }

  // RSA Key Pair Generation
  async generateRsaKeyPair(): Promise<void> {
    try {
      this.isGeneratingKeys.set(true);
      this.panelError.set(null);
      this.keyGenSuccess.set(null);

      const keyPair = await window.crypto.subtle.generateKey(
        {
          name: 'RSASSA-PKCS1-v1_5',
          modulusLength: 2048,
          publicExponent: new Uint8Array([1, 0, 1]),
          hash: 'SHA-256',
        },
        true,
        ['sign', 'verify']
      );

      const spkiBuffer = await window.crypto.subtle.exportKey('spki', keyPair.publicKey);
      const pkcs8Buffer = await window.crypto.subtle.exportKey('pkcs8', keyPair.privateKey);

      const publicKeyPem = this.formatAsPem(this.arrayBufferToBase64(spkiBuffer), 'PUBLIC KEY');
      const privateKeyPem = this.formatAsPem(this.arrayBufferToBase64(pkcs8Buffer), 'RSA PRIVATE KEY');

      this.formData.publicKey = publicKeyPem;
      this.generatedPrivateKey.set(privateKeyPem);
      this.keyGenSuccess.set('RSA 2048-bit key pair generated! Public key populated and private key downloaded.');

      // Automatically trigger private key download
      this.downloadPrivateKey();
    } catch (err: any) {
      console.error('Failed to generate RSA key pair:', err);
      this.panelError.set('Failed to generate RSA key pair: ' + (err.message || err));
    } finally {
      this.isGeneratingKeys.set(false);
    }
  }

  downloadPrivateKey(): void {
    const privKey = this.generatedPrivateKey();
    if (!privKey) return;

    const baseName = this.formData.clientId.trim() ? this.formData.clientId.trim() : 'client';
    const filename = `${baseName}_private_key.pem`;
    const blob = new Blob([privKey], { type: 'application/x-pem-file;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  }

  async copyPrivateKey(): Promise<void> {
    const privKey = this.generatedPrivateKey();
    if (!privKey) return;

    try {
      await navigator.clipboard.writeText(privKey);
      this.keyCopied.set(true);
      setTimeout(() => this.keyCopied.set(false), 2000);
    } catch (err) {
      console.error('Failed to copy private key:', err);
    }
  }

  private arrayBufferToBase64(buffer: ArrayBuffer): string {
    let binary = '';
    const bytes = new Uint8Array(buffer);
    const len = bytes.byteLength;
    for (let i = 0; i < len; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return window.btoa(binary);
  }

  private formatAsPem(base64: string, label: string): string {
    const lines = base64.match(/.{1,64}/g) || [];
    return `-----BEGIN ${label}-----\n${lines.join('\n')}\n-----END ${label}-----`;
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
