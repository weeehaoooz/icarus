import { Component, signal, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { ActivityService } from './services/activity.service';

@Component({
  selector: 'app-root',
  imports: [RouterOutlet],
  templateUrl: './app.html',
  styleUrl: './app.scss'
})
export class App {
  private readonly activityService = inject(ActivityService);
  protected readonly title = signal('iam');
}
