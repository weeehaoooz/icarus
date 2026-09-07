import { Component, signal, inject, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';
import { AdminService } from '../../services/admin.service';

// Talos UI — Layout
import { TalosCardComponent, TalosCardHeaderComponent, TalosCardBodyComponent } from '@weeehaoooz/talos-ui/layout';
// Talos UI — Button
import { TalosButtonDirective } from '@weeehaoooz/talos-ui/button/button';
// Talos UI — Feedback
import { TalosAlertComponent } from '@weeehaoooz/talos-ui/feedback/alert';
import { TalosTooltipDirective } from '@weeehaoooz/talos-ui/feedback/tooltip';
// Talos UI — Data Display
import { TalosStatusTagComponent } from '@weeehaoooz/talos-ui/data-display/status-tag';
import { TalosBadgeDirective } from '@weeehaoooz/talos-ui/data-display/badge';
import { TalosTrendIndicatorComponent } from '@weeehaoooz/talos-ui/data-display/trend-indicator';
// Talos UI — Visualization
import { TalosSparklineComponent } from '@weeehaoooz/talos-ui/data-viz/sparkline';

@Component({
  selector: 'app-overview',
  imports: [
    RouterLink,
    // Layout
    TalosCardComponent,
    TalosCardHeaderComponent,
    TalosCardBodyComponent,
    // Button
    TalosButtonDirective,
    // Feedback
    TalosAlertComponent,
    TalosTooltipDirective,
    // Data Display
    TalosStatusTagComponent,
    TalosBadgeDirective,
    TalosTrendIndicatorComponent,
    // Visualization
    TalosSparklineComponent,
  ],
  templateUrl: './overview.component.html',
  styleUrl: './overview.component.scss'
})
export class OverviewComponent implements OnInit {
  private readonly adminService = inject(AdminService);

  readonly totalUsers = signal(0);
  readonly totalClients = signal(0);
  readonly totalRoles = signal(0);

  // Previous-cycle snapshot values (simulated for trend calculation)
  readonly prevUsers = signal(0);
  readonly prevClients = signal(0);
  readonly prevRoles = signal(0);

  // Simulated sparkline history data (last 8 periods)
  readonly usersSparkline = signal<number[]>([]);
  readonly clientsSparkline = signal<number[]>([]);
  readonly rolesSparkline = signal<number[]>([]);

  ngOnInit(): void {
    this.loadStats();
  }

  loadStats(): void {
    this.adminService.listUsers().subscribe({
      next: (users) => {
        this.totalUsers.set(users.length);
        // Simulate a previous period count (90% of current for demo trend)
        this.prevUsers.set(Math.max(0, Math.floor(users.length * 0.9)));
        this.usersSparkline.set(this.generateSparkline(users.length));
      },
      error: () => {
        this.totalUsers.set(0);
        this.prevUsers.set(0);
      }
    });

    this.adminService.listClients().subscribe({
      next: (clients) => {
        this.totalClients.set(clients.length);
        this.prevClients.set(Math.max(0, Math.floor(clients.length * 0.85)));
        this.clientsSparkline.set(this.generateSparkline(clients.length));
      },
      error: () => {
        this.totalClients.set(0);
        this.prevClients.set(0);
      }
    });

    this.adminService.listRoles().subscribe({
      next: (roles) => {
        this.totalRoles.set(roles.length);
        this.prevRoles.set(Math.max(0, Math.floor(roles.length * 0.95)));
        this.rolesSparkline.set(this.generateSparkline(roles.length));
      },
      error: () => {
        this.totalRoles.set(0);
        this.prevRoles.set(0);
      }
    });
  }

  /** Generate a plausible 8-point sparkline history ending at the given total. */
  private generateSparkline(total: number): number[] {
    const points: number[] = [];
    for (let i = 7; i >= 0; i--) {
      const variance = Math.random() * 0.15 - 0.075; // ±7.5%
      points.push(Math.max(0, Math.round(total * (0.7 + (7 - i) * 0.04 + variance))));
    }
    points.push(total);
    return points;
  }
}
