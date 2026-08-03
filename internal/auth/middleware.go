package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	userIDContextKey  contextKey = "user_id"
	isAdminContextKey contextKey = "is_admin"
)

// RequireAuth validates the Bearer access token and injects the
// authenticated user's id and admin flag into the request context.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		c, err := parseAccessToken(h.jwtSecret, token)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, c.UserID)
		ctx = context.WithValue(ctx, isAdminContextKey, c.IsAdmin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin rejects the request unless RequireAuth already
// authenticated an admin user.
func (h *Handler) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAdmin, _ := r.Context().Value(isAdminContextKey).(bool); !isAdmin {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserIDFromContext returns the authenticated user's id, set by
// RequireAuth.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDContextKey).(string)
	return id, ok
}
