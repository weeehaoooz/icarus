import { Injectable, inject, signal, effect, NgZone, OnDestroy } from '@angular/core';
import { AuthService } from './auth.service';
import { fromEvent, merge, Subscription } from 'rxjs';
import { throttleTime } from 'rxjs/operators';

@Injectable({
  providedIn: 'root'
})
export class ActivityService implements OnDestroy {
  private readonly authService = inject(AuthService);
  private readonly ngZone = inject(NgZone);

  // Inactivity timeout threshold: default to 15 minutes (900,000 milliseconds)
  private readonly IDLE_TIMEOUT_MS = 15 * 60 * 1000;

  private activitySubscription?: Subscription;
  private timeoutId: any = null;

  readonly isIdle = signal(false);

  constructor() {
    // Run the activity monitoring using an effect that reacts to changes in login state
    effect(() => {
      const loggedIn = this.authService.isLoggedIn();
      if (loggedIn) {
        this.startTracking();
      } else {
        this.stopTracking();
      }
    });
  }

  private startTracking(): void {
    this.stopTracking();
    this.isIdle.set(false);

    // Track activity events outside Angular zone to prevent triggering unnecessary change detection cycles
    this.ngZone.runOutsideAngular(() => {
      const mousemove$ = fromEvent(document, 'mousemove').pipe(throttleTime(2000));
      const keydown$ = fromEvent(document, 'keydown').pipe(throttleTime(2000));
      const click$ = fromEvent(document, 'click');
      const scroll$ = fromEvent(window, 'scroll').pipe(throttleTime(2000));

      this.activitySubscription = merge(mousemove$, keydown$, click$, scroll$).subscribe(() => {
        this.resetTimer();
      });

      this.resetTimer();
    });
  }

  private stopTracking(): void {
    if (this.activitySubscription) {
      this.activitySubscription.unsubscribe();
      this.activitySubscription = undefined;
    }
    if (this.timeoutId) {
      clearTimeout(this.timeoutId);
      this.timeoutId = null;
    }
  }

  private resetTimer(): void {
    if (this.timeoutId) {
      clearTimeout(this.timeoutId);
    }

    this.timeoutId = setTimeout(() => {
      this.ngZone.run(() => {
        this.isIdle.set(true);
        // Automatically logout the user due to inactivity
        this.authService.logout();
      });
    }, this.IDLE_TIMEOUT_MS);
  }

  ngOnDestroy(): void {
    this.stopTracking();
  }
}
