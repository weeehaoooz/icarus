import { Routes } from '@angular/router';
import { LoginComponent } from './components/login/login.component';
import { DashboardComponent } from './components/dashboard/dashboard.component';
import { OverviewComponent } from './components/overview/overview.component';
import { UsersComponent } from './components/users/users.component';
import { ClientsComponent } from './components/clients/clients.component';
import { RolesComponent } from './components/roles/roles.component';
import { authGuard } from './guards/auth.guard';

export const routes: Routes = [
  { path: 'login', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardComponent,
    canActivate: [authGuard],
    children: [
      { path: 'overview', component: OverviewComponent },
      { path: 'users', component: UsersComponent },
      { path: 'clients', component: ClientsComponent },
      { path: 'roles', component: RolesComponent },
      { path: 'settings/integrations', loadComponent: () => import('./components/ldap/ldap.component').then(m => m.LdapComponent) },
      { path: 'tenants', loadComponent: () => import('./components/tenants/tenants.component').then(m => m.TenantsComponent) },
      { path: 'modules', loadComponent: () => import('./components/modules/modules.component').then(m => m.ModulesComponent) },
      { path: 'applications', loadComponent: () => import('./components/applications/applications.component').then(m => m.ApplicationsComponent) },
      { path: '', redirectTo: 'overview', pathMatch: 'full' }
    ]
  },
  { path: '', redirectTo: 'dashboard/overview', pathMatch: 'full' },
  { path: '**', redirectTo: 'dashboard/overview' }
];
