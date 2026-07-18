import { Injectable, inject, signal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { Observable, tap, catchError, of, map } from 'rxjs';

export interface DecodedToken {
	sub: string;
	type: string;
	roles?: string[];
	permissions?: string[];
	owned_modules?: string[];
	exp: number;
}

export interface UserSummary {
	id: number;
	username: string;
	first_name: string;
	last_name: string;
}


@Injectable({
	providedIn: 'root'
})
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
			const decoded = JSON.parse(atob(payload)) as DecodedToken;
			return decoded;
		} catch (e) {
			return null;
		}
	});

	readonly isLoggedIn = computed(() => this.currentUser() !== null);
	readonly isAdmin = computed(() => {
		const user = this.currentUser();
		return user?.roles?.includes('admin') || false;
	});
	readonly isModuleOwner = computed(() => {
		const user = this.currentUser();
		return (user?.owned_modules && user.owned_modules.length > 0) || false;
	});

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

	// Try to refresh the access token.
	// Returns the new access token string on success, or null on failure.
	refresh(): Observable<string | null> {
		const refreshTok = this.refreshToken();
		if (!refreshTok) {
			return of(null);
		}

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

	getMyRoles(): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/me/roles`);
	}

	getMyPermissions(): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/me/permissions`);
	}

	updateProfile(profile: any): Observable<any> {
		return this.http.put<any>(`${this.apiUrl}/me`, profile);
	}

	changePassword(payload: any): Observable<any> {
		return this.http.put<any>(`${this.apiUrl}/me/password`, payload);
	}

	/** Returns the user directory — safe fields only (id, username, first_name, last_name). */
	listUsers(): Observable<UserSummary[]> {
		return this.http.get<UserSummary[]>(`${this.apiUrl}/users`);
	}

	/** Returns the detailed role list for any user by their numeric ID. */
	getUserRoles(userId: number): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/users/${userId}/roles`);
	}
}

