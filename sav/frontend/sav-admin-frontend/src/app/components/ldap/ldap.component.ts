import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { AdminService } from '../../services/admin.service';

@Component({
  selector: 'app-ldap',
  imports: [CommonModule, FormsModule],
  templateUrl: './ldap.component.html',
  styleUrl: './ldap.component.scss'
})
export class LdapComponent implements OnInit {
  private readonly adminService = inject(AdminService);

  // Configuration Signals
  protected readonly enabled = signal<boolean>(false);
  protected readonly server_url = signal<string>('');
  protected readonly bind_dn = signal<string>('');
  protected readonly bind_password = signal<string>('');
  protected readonly search_base = signal<string>('');
  protected readonly username_attribute = signal<string>('uid');
  protected readonly mail_attribute = signal<string>('mail');
  protected readonly first_name_attribute = signal<string>('givenName');
  protected readonly last_name_attribute = signal<string>('sn');

  // Interactive States
  protected readonly isTesting = signal<boolean>(false);
  protected readonly isSaving = signal<boolean>(false);
  protected readonly message = signal<{ text: string; type: 'success' | 'error' | null }>({ text: '', type: null });

  ngOnInit(): void {
    this.loadConfig();
  }

  loadConfig(): void {
    this.adminService.getLDAPConfig().subscribe({
      next: (config) => {
        this.enabled.set(config.enabled);
        this.server_url.set(config.server_url || '');
        this.bind_dn.set(config.bind_dn || '');
        this.bind_password.set(config.bind_password || '');
        this.search_base.set(config.search_base || '');
        this.username_attribute.set(config.username_attribute || 'uid');
        this.mail_attribute.set(config.mail_attribute || 'mail');
        this.first_name_attribute.set(config.first_name_attribute || 'givenName');
        this.last_name_attribute.set(config.last_name_attribute || 'sn');
      },
      error: (err) => {
        this.showNotification('Failed to load LDAP configuration: ' + (err.error?.error || err.message), 'error');
      }
    });
  }

  private getConfigPayload() {
    return {
      enabled: this.enabled(),
      server_url: this.server_url(),
      bind_dn: this.bind_dn(),
      bind_password: this.bind_password(),
      search_base: this.search_base(),
      username_attribute: this.username_attribute(),
      mail_attribute: this.mail_attribute(),
      first_name_attribute: this.first_name_attribute(),
      last_name_attribute: this.last_name_attribute()
    };
  }

  testConnection(): void {
    this.isTesting.set(true);
    this.clearNotification();
    this.adminService.testLDAPConfig(this.getConfigPayload()).subscribe({
      next: (res) => {
        this.isTesting.set(false);
        if (res.success) {
          this.showNotification(res.message || 'LDAP connection test successful!', 'success');
        } else {
          this.showNotification('LDAP connection test failed: ' + res.error, 'error');
        }
      },
      error: (err) => {
        this.isTesting.set(false);
        this.showNotification('LDAP connection test failed: ' + (err.error?.error || err.message), 'error');
      }
    });
  }

  saveConfig(): void {
    this.isSaving.set(true);
    this.clearNotification();
    this.adminService.updateLDAPConfig(this.getConfigPayload()).subscribe({
      next: (res) => {
        this.isSaving.set(false);
        this.showNotification(res.message || 'LDAP configuration saved successfully!', 'success');
        this.loadConfig();
      },
      error: (err) => {
        this.isSaving.set(false);
        this.showNotification('Failed to save LDAP configuration: ' + (err.error?.error || err.message), 'error');
      }
    });
  }

  toggleEnabled(): void {
    this.enabled.update(val => !val);
  }

  private showNotification(text: string, type: 'success' | 'error'): void {
    this.message.set({ text, type });
  }

  private clearNotification(): void {
    this.message.set({ text: '', type: null });
  }
}
