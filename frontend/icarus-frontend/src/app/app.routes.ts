import { Routes } from '@angular/router';
import { LoginComponent } from './components/login/login.component';
import { DashboardComponent } from './components/dashboard/dashboard.component';
import { OverviewComponent } from './components/overview/overview.component';
import { UsersComponent } from './components/users/users.component';
import { ClientsComponent } from './components/clients/clients.component';
import { RolesComponent } from './components/roles/roles.component';
import { authGuard } from './guards/auth.guard';
import { adminGuard } from './guards/admin.guard';

export const routes: Routes = [
  { path: 'login', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardComponent,
    canActivate: [authGuard],
    children: [
      { path: 'overview', component: OverviewComponent, canActivate: [adminGuard] },
      { path: 'users', component: UsersComponent, canActivate: [adminGuard] },
      { path: 'clients', component: ClientsComponent, canActivate: [adminGuard] },
      { path: 'roles', component: RolesComponent, canActivate: [adminGuard] },
      { path: 'settings/integrations', loadComponent: () => import('./components/ldap/ldap.component').then(m => m.LdapComponent), canActivate: [adminGuard] },
      { path: 'tenants', loadComponent: () => import('./components/tenants/tenants.component').then(m => m.TenantsComponent), canActivate: [adminGuard] },
      { path: 'modules', loadComponent: () => import('./components/modules/modules.component').then(m => m.ModulesComponent) },
      { path: 'applications', loadComponent: () => import('./components/applications/applications.component').then(m => m.ApplicationsComponent), canActivate: [adminGuard] },
      { path: 'my-policies', loadComponent: () => import('./components/my-policies/my-policies.component').then(m => m.MyPoliciesComponent) },
      { path: 'policy-sandbox', loadComponent: () => import('./components/policy-sandbox/policy-sandbox.component').then(m => m.PolicySandboxComponent) },
      { path: 'request-access', loadComponent: () => import('./components/request-access/request-access.component').then(m => m.RequestAccessComponent) },
      { path: 'approval-inbox', loadComponent: () => import('./components/approval-inbox/approval-inbox.component').then(m => m.ApprovalInboxComponent) },
      { path: 'request-detail/:instanceId', loadComponent: () => import('./components/request-detail/request-detail.component').then(m => m.RequestDetailComponent) },
      { path: 'workflow-builder/:roleId', loadComponent: () => import('./components/workflow-builder/workflow-builder.component').then(m => m.WorkflowBuilderComponent), canActivate: [adminGuard] },
      { path: 'profile-settings', loadComponent: () => import('./components/profile-settings/profile-settings.component').then(m => m.ProfileSettingsComponent) },
      { path: '', redirectTo: 'my-policies', pathMatch: 'full' }
    ]
  },
  { path: '', redirectTo: 'dashboard/my-policies', pathMatch: 'full' },
  { path: '**', redirectTo: 'dashboard/my-policies' }
];
