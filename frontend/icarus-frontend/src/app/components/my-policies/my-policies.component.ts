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
	selector: 'app-my-policies',
	imports: [FormsModule],
	templateUrl: './my-policies.component.html',
	styleUrl: './my-policies.component.scss'
})
export class MyPoliciesComponent implements OnInit {
	private readonly authService = inject(AuthService);
	private readonly adminService = inject(AdminService);
	private readonly platformService = inject(PlatformService);

	// Tabs: 'overview' | 'sandbox' | 'requests' | 'profile'
	readonly activeTab = signal<'overview' | 'sandbox' | 'requests' | 'profile'>('overview');

	// Current User Profile Signals
	readonly userProfile = signal<UserProfile | null>(null);
	readonly isLoadingProfile = signal(false);
	readonly profileError = signal<string | null>(null);

	// Core Metadata Signals
	readonly roles = signal<Role[]>([]);
	readonly permissions = signal<Permission[]>([]);
	readonly modules = signal<any[]>([]);
	readonly tenants = signal<any[]>([]);

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

	// Access Request Form
	requestForm = {
		tenant: 'system-tenant',
		module: 'icarus-auth-ms',
		role: '',
		justification: ''
	};
	readonly requestSuccessMessage = signal<string | null>(null);
	readonly requestErrorMessage = signal<string | null>(null);
	readonly requestHistory = signal<AccessRequest[]>([]);

	// Sandbox Simulator State
	sandboxForm = {
		module: 'icarus-auth-ms',
		action: ''
	};
	readonly isEvaluating = signal(false);
	readonly evaluationResult = signal<'GRANTED' | 'DENIED' | null>(null);
	readonly evaluationTrace = signal<string[]>([]);

	// Derived: Filtered actions for selected module in sandbox
	readonly sandboxActions = computed(() => {
		const modId = this.sandboxForm.module;
		return this.permissions()
			.filter(p => p.module_id === modId)
			.map(p => p.action);
	});

	// Derived: Roles available for user to request (exclude already assigned)
	readonly requestableRoles = computed(() => {
		const currentRoles = this.userProfile()?.roles || [];
		return this.roles().filter(r => !currentRoles.includes(r.name));
	});

	// Derived: Active User Roles with details
	readonly myActiveRoles = computed(() => {
		const currentRoleNames = this.userProfile()?.roles || [];
		const allRoles = this.roles();
		return allRoles.filter(r => currentRoleNames.includes(r.name));
	});

	// Derived: Resolved Permissions for current user based on role hierarchy
	readonly myResolvedPermissions = computed(() => {
		const currentRoleNames = this.userProfile()?.roles || [];
		const allRoles = this.roles();
		const resolvedPermissionsSet = new Set<string>();
		const visitedRoles = new Set<string>();

		const traverse = (roleName: string) => {
			if (visitedRoles.has(roleName)) return;
			visitedRoles.add(roleName);

			const roleObj = allRoles.find(r => r.name === roleName);
			if (!roleObj) return;

			if (roleObj.permissions) {
				roleObj.permissions.forEach(p => resolvedPermissionsSet.add(p));
			}

			if (roleObj.nested_roles) {
				roleObj.nested_roles.forEach(nr => traverse(nr));
			}
		};

		currentRoleNames.forEach(rn => traverse(rn));
		return Array.from(resolvedPermissionsSet);
	});

	// Derived: Resolved Permission Details (joins permission objects)
	readonly myResolvedPermissionDetails = computed(() => {
		const actions = this.myResolvedPermissions();
		const allPerms = this.permissions();
		return allPerms.filter(p => actions.includes(p.module_id + ':' + p.action) || actions.includes(p.action));
	});

	ngOnInit(): void {
		this.loadUserProfile();
		this.loadSystemData();
		this.loadRequestHistory();
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

	loadSystemData(): void {
		this.adminService.listRoles().subscribe({
			next: (data) => this.roles.set(data as Role[]),
			error: (err) => console.error('Failed to load system roles:', err)
		});

		this.adminService.listPermissions().subscribe({
			next: (data) => {
				this.permissions.set(data as Permission[]);
				// Set default sandbox action if available
				const actions = this.sandboxActions();
				if (actions.length > 0) {
					this.sandboxForm.action = actions[0];
				}
			},
			error: (err) => console.error('Failed to load system permissions:', err)
		});

		this.platformService.listModules().subscribe({
			next: (data) => this.modules.set(data),
			error: (err) => console.error('Failed to load system modules:', err)
		});

		this.platformService.listTenants().subscribe({
			next: (data) => this.tenants.set(data),
			error: (err) => console.error('Failed to load system tenants:', err)
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

	onSandboxModuleChange(): void {
		const actions = this.sandboxActions();
		this.sandboxForm.action = actions.length > 0 ? actions[0] : '';
	}

	switchTab(tab: 'overview' | 'sandbox' | 'requests' | 'profile'): void {
		this.activeTab.set(tab);
		// Reset temporary messages
		this.profileSuccessMessage.set(null);
		this.profileErrorMessage.set(null);
		this.passwordSuccessMessage.set(null);
		this.passwordErrorMessage.set(null);
		this.requestSuccessMessage.set(null);
		this.requestErrorMessage.set(null);
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

		// Save to history
		const history = [newRequest, ...this.requestHistory()];
		this.requestHistory.set(history);
		localStorage.setItem('access_requests', JSON.stringify(history));

		// If user is Admin, auto-approve and write directly to database
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

			// Hierarchy traversal
			const allRoles = this.roles();
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

			// Match
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
