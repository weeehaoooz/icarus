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

let isRefreshing = false;
const refreshDone$ = new BehaviorSubject<string | null>(null);

function addToken(req: HttpRequest<unknown>, token: string): HttpRequest<unknown> {
  return req.clone({ setHeaders: { Authorization: `Bearer ${token}` } });
}

export const authInterceptor: HttpInterceptorFn = (
  req: HttpRequest<unknown>,
  next: HttpHandlerFn
): Observable<any> => {
  const authService = inject(AuthService);
  const isAuthEndpoint = req.url.includes('/login') || req.url.includes('/refresh');
  const token = authService.accessToken();
  const outgoing = token && !isAuthEndpoint ? addToken(req, token) : req;

  return next(outgoing).pipe(
    catchError((error: unknown) => {
      if (error instanceof HttpErrorResponse && error.status === 401 && !isAuthEndpoint) {
        return handle401(req, next, authService);
      }
      return throwError(() => error);
    })
  );
};

function handle401(
  req: HttpRequest<unknown>,
  next: HttpHandlerFn,
  authService: AuthService
): Observable<any> {
  if (!isRefreshing) {
    isRefreshing = true;
    refreshDone$.next(null);

    return authService.refresh().pipe(
      switchMap((newToken: string | null) => {
        isRefreshing = false;
        refreshDone$.next(newToken);
        if (newToken) return next(addToken(req, newToken));
        return throwError(() => new Error('Session expired'));
      }),
      catchError(err => {
        isRefreshing = false;
        refreshDone$.next(null);
        return throwError(() => err);
      })
    );
  }

  return refreshDone$.pipe(
    filter(token => token !== null),
    take(1),
    switchMap(token => next(addToken(req, token!)))
  );
}
