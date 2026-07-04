import { inject } from '@angular/core';
import {
  HttpInterceptorFn,
  HttpRequest,
  HttpHandlerFn,
  HttpErrorResponse,
} from '@angular/common/http';
import { throwError, BehaviorSubject, Observable } from 'rxjs';
import { catchError, filter, take, switchMap, map } from 'rxjs/operators';
import { AuthService } from '../services/auth.service';

/**
 * A shared lock so that concurrent 401 failures only trigger a single refresh call.
 * These are module-level singletons because functional interceptors share the same scope.
 */
let isRefreshing = false;
const refreshDone$ = new BehaviorSubject<string | null>(null);

/**
 * Clones the request and attaches the given Bearer token.
 */
function addToken(req: HttpRequest<unknown>, token: string): HttpRequest<unknown> {
  return req.clone({
    setHeaders: { Authorization: `Bearer ${token}` },
  });
}

export const authInterceptor: HttpInterceptorFn = (
  req: HttpRequest<unknown>,
  next: HttpHandlerFn
): Observable<any> => {
  const authService = inject(AuthService);

  // Skip attaching tokens for auth endpoints themselves
  const isAuthEndpoint =
    req.url.includes('/login') || req.url.includes('/refresh');

  const token = authService.accessToken();
  const outgoing = token && !isAuthEndpoint ? addToken(req, token) : req;

  return next(outgoing).pipe(
    catchError((error: unknown) => {
      if (
        error instanceof HttpErrorResponse &&
        error.status === 401 &&
        !isAuthEndpoint
      ) {
        return handle401(req, next, authService);
      }
      return throwError(() => error);
    })
  );
};

/**
 * Handles a 401 by performing (at most) one token refresh, then replaying the
 * original request with the new access token. Any concurrent 401s wait for the
 * refresh to complete instead of issuing duplicate refresh calls.
 */
function handle401(
  req: HttpRequest<unknown>,
  next: HttpHandlerFn,
  authService: AuthService
): Observable<any> {
  if (!isRefreshing) {
    isRefreshing = true;
    refreshDone$.next(null); // lock

    return authService.refresh().pipe(
      switchMap((newToken: string | null) => {
        isRefreshing = false;
        refreshDone$.next(newToken); // broadcast new token to queued requests
        if (newToken) {
          return next(addToken(req, newToken));
        }
        // refresh() already called logout() for us
        return throwError(() => new Error('Session expired'));
      }),
      catchError((err) => {
        isRefreshing = false;
        refreshDone$.next(null);
        return throwError(() => err);
      })
    );
  }

  // Another request is already refreshing — queue until we get the new token
  return refreshDone$.pipe(
    filter((token) => token !== null),
    take(1),
    switchMap((token) => next(addToken(req, token!)))
  );
}
