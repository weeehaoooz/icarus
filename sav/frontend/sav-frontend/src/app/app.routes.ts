import { Routes } from '@angular/router';
import { authGuard } from './guards/auth.guard';

export const routes: Routes = [
  {
    path: 'login',
    loadComponent: () => import('./components/login/login.component').then(m => m.LoginComponent)
  },
  {
    path: '',
    loadComponent: () => import('./components/shell/shell.component').then(m => m.ShellComponent),
    canActivate: [authGuard],
    children: [
      {
        path: 'dashboard',
        loadComponent: () => import('./components/dashboard/dashboard.component').then(m => m.DashboardComponent)
      },
      {
        path: 'assets',
        loadComponent: () => import('./components/assets/assets.component').then(m => m.AssetsComponent)
      },
      {
        path: 'liabilities',
        loadComponent: () => import('./components/liabilities/liabilities.component').then(m => m.LiabilitiesComponent)
      },
      {
        path: 'insights',
        loadComponent: () => import('./components/insights/insights.component').then(m => m.InsightsComponent)
      },
      {
        path: 'settings',
        loadComponent: () => import('./components/settings/settings.component').then(m => m.SettingsComponent)
      },
      { path: '', redirectTo: 'dashboard', pathMatch: 'full' }
    ]
  },
  { path: '**', redirectTo: 'dashboard' }
];
