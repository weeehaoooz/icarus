import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../services/auth.service';

export const adminGuard: CanActivateFn = (route, state) => {
	const authService = inject(AuthService);
	const router = inject(Router);

	if (authService.isLoggedIn() && (authService.isAdmin() || authService.isModuleOwner())) {
		return true;
	}

	// Redirect non-admin users to their self-service policies view
	router.navigate(['/dashboard/my-policies']);
	return false;
};
