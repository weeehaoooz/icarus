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
      { path: 'modules', loadComponent: () => import('./components/modules/modules.component').then(m => m.ModulesComponent), canActivate: [adminGuard] },
      { path: 'applications', loadComponent: () => import('./components/applications/applications.component').then(m => m.ApplicationsComponent), canActivate: [adminGuard] },
      { path: 'my-policies', loadComponent: () => import('./components/my-policies/my-policies.component').then(m => m.MyPoliciesComponent) },
      { path: '', redirectTo: 'my-policies', pathMatch: 'full' }
    ]
  },
  { path: '', redirectTo: 'dashboard/my-policies', pathMatch: 'full' },
  { path: '**', redirectTo: 'dashboard/my-policies' }
];
