import { Routes } from '@angular/router';
import { LoginComponent } from './components/login/login.component';
import { DashboardComponent } from './components/dashboard/dashboard.component';
import { authGuard } from './guards/auth.guard';
import { adminGuard } from './guards/admin.guard';

export const routes: Routes = [
  { path: 'login', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardComponent,
    canActivate: [authGuard],
    children: [
      { path: 'overview', loadComponent: () => import('./components/overview/overview.component').then(m => m.OverviewComponent), canActivate: [adminGuard] },
      { path: 'users', loadComponent: () => import('./components/users/users.component').then(m => m.UsersComponent), canActivate: [adminGuard] },
      { path: 'clients', loadComponent: () => import('./components/clients/clients.component').then(m => m.ClientsComponent), canActivate: [adminGuard] },
      { path: 'roles', loadComponent: () => import('./components/roles/roles.component').then(m => m.RolesComponent), canActivate: [adminGuard] },
      { path: 'settings/integrations', loadComponent: () => import('./components/ldap/ldap.component').then(m => m.LdapComponent), canActivate: [adminGuard] },
      { path: 'tenants', loadComponent: () => import('./components/tenants/tenants.component').then(m => m.TenantsComponent), canActivate: [adminGuard] },
      { path: 'modules', loadComponent: () => import('./components/modules/modules.component').then(m => m.ModulesComponent) },
      { path: 'applications', loadComponent: () => import('./components/applications/applications.component').then(m => m.ApplicationsComponent), canActivate: [adminGuard] },
      { path: 'my-policies', loadComponent: () => import('./components/my-policies/my-policies.component').then(m => m.MyPoliciesComponent) },
      { path: 'request-access', loadComponent: () => import('./components/request-access/request-access.component').then(m => m.RequestAccessComponent) },
      { path: 'my-requests', loadComponent: () => import('./components/my-requests/my-requests.component').then(m => m.MyRequestsComponent) },
      { path: 'compare-access', loadComponent: () => import('./components/compare-access/compare-access.component').then(m => m.CompareAccessComponent) },
      { path: 'approval-inbox', loadComponent: () => import('./components/approval-inbox/approval-inbox.component').then(m => m.ApprovalInboxComponent) },
      { path: 'request-detail/:instanceId', loadComponent: () => import('./components/request-detail/request-detail.component').then(m => m.RequestDetailComponent) },
      { path: 'requests/:instanceId', loadComponent: () => import('./components/request-detail/request-detail.component').then(m => m.RequestDetailComponent) },
      { path: 'request-management', loadComponent: () => import('./components/request-management/request-management.component').then(m => m.RequestManagementComponent), canActivate: [adminGuard] },
      { path: 'workflows', loadComponent: () => import('./components/workflow-templates/workflow-templates.component').then(m => m.WorkflowTemplatesComponent), canActivate: [adminGuard] },
      { path: 'workflows/:id', loadComponent: () => import('./components/workflow-details/workflow-details.component').then(m => m.WorkflowDetailsComponent), canActivate: [adminGuard] },
      { path: 'workflow-builder/new', loadComponent: () => import('./components/workflow-builder/workflow-builder.component').then(m => m.WorkflowBuilderComponent), canActivate: [adminGuard] },
      { path: 'workflow-builder/:roleId', loadComponent: () => import('./components/workflow-builder/workflow-builder.component').then(m => m.WorkflowBuilderComponent), canActivate: [adminGuard] },
      { path: 'profile-settings', loadComponent: () => import('./components/profile-settings/profile-settings.component').then(m => m.ProfileSettingsComponent) },
      { path: '', redirectTo: 'my-policies', pathMatch: 'full' }
    ]
  },
  { path: '', redirectTo: 'dashboard/my-policies', pathMatch: 'full' },
  { path: '**', redirectTo: 'dashboard/my-policies' }
];
