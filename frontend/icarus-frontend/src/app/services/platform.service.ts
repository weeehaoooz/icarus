import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

@Injectable({
	providedIn: 'root'
})
export class PlatformService {
	private readonly http = inject(HttpClient);
	private readonly apiUrl = 'http://localhost:8081/admin';

	// === Tenants ===
	listTenants(): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/tenants`);
	}

	createTenant(tenant: any): Observable<any> {
		return this.http.post<any>(`${this.apiUrl}/tenants`, tenant);
	}

	updateTenant(id: string, tenant: any): Observable<any> {
		return this.http.put<any>(`${this.apiUrl}/tenants/${id}`, tenant);
	}

	deleteTenant(id: string): Observable<any> {
		return this.http.delete<any>(`${this.apiUrl}/tenants/${id}`);
	}

	// === Modules ===
	listModules(): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/modules`);
	}

	createModule(module: any): Observable<any> {
		return this.http.post<any>(`${this.apiUrl}/modules`, module);
	}

	updateModule(id: string, module: any): Observable<any> {
		return this.http.put<any>(`${this.apiUrl}/modules/${id}`, module);
	}

	deleteModule(id: string): Observable<any> {
		return this.http.delete<any>(`${this.apiUrl}/modules/${id}`);
	}

	getModuleManifest(id: string): Observable<any> {
		return this.http.get<any>(`${this.apiUrl}/modules/${id}/manifest`);
	}

	// === Applications ===
	listApplications(): Observable<any[]> {
		return this.http.get<any[]>(`${this.apiUrl}/applications`);
	}

	createApplication(app: any): Observable<any> {
		return this.http.post<any>(`${this.apiUrl}/applications`, app);
	}

	updateApplication(id: string, app: any): Observable<any> {
		return this.http.put<any>(`${this.apiUrl}/applications/${id}`, app);
	}

	deleteApplication(id: string): Observable<any> {
		return this.http.delete<any>(`${this.apiUrl}/applications/${id}`);
	}

	// === Module Application Onboarding ===
	listModuleApplications(moduleId: string): Observable<string[]> {
		return this.http.get<string[]>(`${this.apiUrl}/modules/${moduleId}/applications`);
	}

	onboardApplicationToModule(moduleId: string, appCode: string): Observable<any> {
		return this.http.post<any>(`${this.apiUrl}/modules/${moduleId}/applications`, { app_code: appCode });
	}

	offboardApplicationFromModule(moduleId: string, appCode: string): Observable<any> {
		return this.http.delete<any>(`${this.apiUrl}/modules/${moduleId}/applications/${appCode}`);
	}
}
