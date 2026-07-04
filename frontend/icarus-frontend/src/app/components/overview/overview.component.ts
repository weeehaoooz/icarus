import { Component, signal, inject, OnInit } from '@angular/core';
import { RouterLink } from '@angular/router';
import { AdminService } from '../../services/admin.service';

@Component({
  selector: 'app-overview',
  imports: [RouterLink],
  templateUrl: './overview.component.html',
  styleUrl: './overview.component.scss'
})
export class OverviewComponent implements OnInit {
  private readonly adminService = inject(AdminService);

  readonly totalUsers = signal(0);
  readonly totalClients = signal(0);
  readonly totalRoles = signal(0);

  ngOnInit(): void {
    this.loadStats();
  }

  loadStats(): void {
    this.adminService.listUsers().subscribe({
      next: (users) => this.totalUsers.set(users.length),
      error: () => this.totalUsers.set(0)
    });

    this.adminService.listClients().subscribe({
      next: (clients) => this.totalClients.set(clients.length),
      error: () => this.totalClients.set(0)
    });

    this.adminService.listRoles().subscribe({
      next: (roles) => this.totalRoles.set(roles.length),
      error: () => this.totalRoles.set(0)
    });
  }
}
