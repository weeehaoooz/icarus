import { Component, signal, inject, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AuthService } from '../../services/auth.service';

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

@Component({
  selector: 'app-profile-settings',
  imports: [FormsModule],
  templateUrl: './profile-settings.component.html',
  styleUrl: './profile-settings.component.scss'
})
export class ProfileSettingsComponent implements OnInit {
  private readonly authService = inject(AuthService);

  readonly userProfile = signal<UserProfile | null>(null);
  readonly isLoadingProfile = signal(false);
  readonly profileError = signal<string | null>(null);

  // Profile Form
  profileForm = {
    firstName: '',
    lastName: '',
    email: ''
  };
  readonly profileSuccessMessage = signal<string | null>(null);
  readonly profileErrorMessage = signal<string | null>(null);

  // Password Form
  passwordForm = {
    oldPassword: '',
    newPassword: '',
    confirmPassword: ''
  };
  readonly passwordSuccessMessage = signal<string | null>(null);
  readonly passwordErrorMessage = signal<string | null>(null);

  ngOnInit(): void {
    this.loadUserProfile();
  }

  loadUserProfile(): void {
    this.isLoadingProfile.set(true);
    this.profileError.set(null);
    this.authService.getProfile().subscribe({
      next: (data) => {
        this.userProfile.set(data);
        this.profileForm = {
          firstName: data.first_name || '',
          lastName: data.last_name || '',
          email: data.email || ''
        };
        this.isLoadingProfile.set(false);
      },
      error: (err) => {
        this.profileError.set('Failed to load profile details.');
        this.isLoadingProfile.set(false);
      }
    });
  }

  updateProfile(): void {
    this.profileSuccessMessage.set(null);
    this.profileErrorMessage.set(null);

    this.authService.updateProfile({
      first_name: this.profileForm.firstName,
      last_name: this.profileForm.lastName,
      email: this.profileForm.email
    }).subscribe({
      next: () => {
        this.profileSuccessMessage.set('Profile updated successfully.');
        this.loadUserProfile();
      },
      error: (err) => {
        this.profileErrorMessage.set(err.error?.error || 'Failed to update profile.');
      }
    });
  }

  changePassword(): void {
    this.passwordSuccessMessage.set(null);
    this.passwordErrorMessage.set(null);

    if (this.passwordForm.newPassword !== this.passwordForm.confirmPassword) {
      this.passwordErrorMessage.set('New passwords do not match.');
      return;
    }

    this.authService.changePassword({
      old_password: this.passwordForm.oldPassword,
      new_password: this.passwordForm.newPassword
    }).subscribe({
      next: () => {
        this.passwordSuccessMessage.set('Password changed successfully.');
        this.passwordForm = { oldPassword: '', newPassword: '', confirmPassword: '' };
      },
      error: (err) => {
        this.passwordErrorMessage.set(err.error?.error || 'Failed to change password.');
      }
    });
  }
}
