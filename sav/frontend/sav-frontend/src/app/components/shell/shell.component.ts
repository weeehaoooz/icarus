import { Component, inject, signal } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive } from '@angular/router';
import { AuthService } from '../../services/auth.service';
import { ThemeService } from '../../services/theme.service';

@Component({
  selector: 'app-shell',
  imports: [RouterOutlet, RouterLink, RouterLinkActive],
  templateUrl: './shell.component.html',
  styleUrl: './shell.component.scss'
})
export class ShellComponent {
  protected readonly auth = inject(AuthService);
  protected readonly theme = inject(ThemeService);
  protected readonly sidebarCollapsed = signal(false);

  toggleSidebar(): void {
    this.sidebarCollapsed.update(v => !v);
  }

  readonly navItems = [
    { path: '/dashboard',    icon: '⬡', label: 'Dashboard'    },
    { path: '/assets',       icon: '◈', label: 'Assets'       },
    { path: '/liabilities',  icon: '◎', label: 'Liabilities'  },
    { path: '/insights',     icon: '◆', label: 'Insights'     },
    { path: '/settings',     icon: '⚙', label: 'Settings'     },
  ];
}
