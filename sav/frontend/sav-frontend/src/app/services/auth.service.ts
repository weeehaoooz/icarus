import { Injectable, inject, signal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { Observable, tap, catchError, of, map } from 'rxjs';

export interface DecodedToken {
  sub: string;
  type: string;
  roles?: string[];
  permissions?: string[];
  exp: number;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly router = inject(Router);
  private readonly apiUrl = 'http://localhost:8080';

  // Signals for state
  readonly accessToken = signal<string | null>(localStorage.getItem('access_token'));
  readonly refreshToken = signal<string | null>(localStorage.getItem('refresh_token'));

  readonly currentUser = computed(() => {
    const token = this.accessToken();
    if (!token) return null;
    try {
      const payload = token.split('.')[1];
      return JSON.parse(atob(payload)) as DecodedToken;
    } catch {
      return null;
    }
  });

  readonly isLoggedIn = computed(() => this.currentUser() !== null);

  // No admin gate — any authenticated user can access the finance app
  readonly userSub = computed(() => this.currentUser()?.sub ?? null);

  login(username: string, password: string): Observable<any> {
    return this.http.post<any>(`${this.apiUrl}/login`, { username, password }).pipe(
      tap(res => {
        if (res.access_token && res.refresh_token) {
          localStorage.setItem('access_token', res.access_token);
          localStorage.setItem('refresh_token', res.refresh_token);
          this.accessToken.set(res.access_token);
          this.refreshToken.set(res.refresh_token);
        }
      })
    );
  }

  logout(): void {
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
    this.accessToken.set(null);
    this.refreshToken.set(null);
    this.router.navigate(['/login']);
  }

  getAuthHeaders(): { Authorization: string } {
    const token = this.accessToken();
    return { Authorization: token ? `Bearer ${token}` : '' };
  }

  refresh(): Observable<string | null> {
    const refreshTok = this.refreshToken();
    if (!refreshTok) return of(null);

    return this.http.post<any>(`${this.apiUrl}/refresh`, { refresh_token: refreshTok }).pipe(
      tap(res => {
        if (res.access_token && res.refresh_token) {
          localStorage.setItem('access_token', res.access_token);
          localStorage.setItem('refresh_token', res.refresh_token);
          this.accessToken.set(res.access_token);
          this.refreshToken.set(res.refresh_token);
        }
      }),
      map(res => res.access_token ?? null),
      catchError(() => {
        this.logout();
        return of(null);
      })
    );
  }

  getProfile(): Observable<any> {
    return this.http.get<any>(`${this.apiUrl}/me`);
  }

  updateProfile(profile: { email: string; first_name: string; last_name: string }): Observable<any> {
    return this.http.put<any>(`${this.apiUrl}/me`, profile);
  }

  changePassword(passwords: { old_password: string; new_password: string }): Observable<any> {
    return this.http.put<any>(`${this.apiUrl}/me/password`, passwords);
  }
}
