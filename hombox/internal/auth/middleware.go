package auth

import (
	"context"
	"log/slog"
	"net/http"
)

// Middleware validates the session cookie and injects user ID into the request context.
func Middleware(svc *Service) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := GetSessionCookie(r)
			if token == "" {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			userID, err := svc.ValidateSession(r.Context(), token)
			if err != nil {
				slog.Debug("session validation failed", "error", err)
				ClearSessionCookie(w)
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			ctx = context.WithValue(ctx, SessionToken, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth validates the session if present, but doesn't require it.
// Useful for endpoints that work both authenticated and anonymous.
func OptionalAuth(svc *Service) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := GetSessionCookie(r)
			if token != "" {
				userID, err := svc.ValidateSession(r.Context(), token)
				if err == nil {
					ctx := context.WithValue(r.Context(), UserIDKey, userID)
					ctx = context.WithValue(ctx, SessionToken, token)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
