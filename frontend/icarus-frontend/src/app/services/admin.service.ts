import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';

@Injectable({
	providedIn: 'root'
})
export class AdminService {
	private readonly http = inject(HttpClient);
	private readonly apiUrl = 'http://localhost:8080/admin';

	// === Users ===
	listUsers(): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/users`);
	}

	createUser(user: any): Observable<any> {
		return this.http.post<any>(`${this.apiUrl}/users`, user);
	}

	getUser(id: number): Observable<any> {
		return this.http.get<any>(`${this.apiUrl}/users/${id}`);
	}

	updateUser(id: number, user: any): Observable<any> {
		return this.http.put<any>(`${this.apiUrl}/users/${id}`, user);
	}

	deleteUser(id: number): Observable<any> {
		return this.http.delete<any>(`${this.apiUrl}/users/${id}`);
	}

	// === Clients ===
	listClients(): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/clients`);
	}

	createClient(client: any): Observable<any> {
		return this.http.post<any>(`${this.apiUrl}/clients`, client);
	}

	getClient(id: string): Observable<any> {
		return this.http.get<any>(`${this.apiUrl}/clients/${id}`);
	}

	updateClient(id: string, client: any): Observable<any> {
		return this.http.put<any>(`${this.apiUrl}/clients/${id}`, client);
	}

	deleteClient(id: string): Observable<any> {
		return this.http.delete<any>(`${this.apiUrl}/clients/${id}`);
	}

	// === Roles & Permissions ===
	listRoles(search?: string, limit?: number, offset?: number): Observable<any[]> {
		let params = new HttpParams();
		if (search) {
			params = params.set('q', search);
		}
		if (limit !== undefined) {
			params = params.set('limit', limit.toString());
		}
		if (offset !== undefined) {
			params = params.set('offset', offset.toString());
		}
		return this.http.get<any[]>('http://localhost:8080/roles', { params });
	}

	createRole(role: any): Observable<any> {
		return this.http.post<any>(`${this.apiUrl}/roles`, role);
	}

	updateRole(id: string, role: any): Observable<any> {
		return this.http.put<any>(`${this.apiUrl}/roles/${id}`, role);
	}

	deleteRole(id: string): Observable<any> {
		return this.http.delete<any>(`${this.apiUrl}/roles/${id}`);
	}

	listPermissions(): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/permissions`);
	}

	listApplications(): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/applications`);
	}

	// === LDAP Integration ===
	getLDAPConfig(): Observable<any> {
		return this.http.get<any>(`${this.apiUrl}/ldap-config`);
	}

	updateLDAPConfig(config: any): Observable<any> {
		return this.http.put<any>(`${this.apiUrl}/ldap-config`, config);
	}

	testLDAPConfig(config: any): Observable<any> {
		return this.http.post<any>(`${this.apiUrl}/ldap-config/test`, config);
	}
}
