package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jorgebuitragor/logday-server/internal/httperr"
)

type contextKey string

const (
	userIDContextKey  contextKey = "user_id"
	isAdminContextKey contextKey = "is_admin"
)

// errDeviceRevoked means the access token is well signed and unexpired,
// but the device it was issued to no longer exists: logged out, revoked
// from the panel or by the user, or its user was deleted.
var errDeviceRevoked = errors.New("device revoked")

// RequireAuth validates the Bearer access token and injects the
// authenticated user's id and admin flag into the request context.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		c, err := h.verifyAccessToken(r.Context(), token)
		if errors.Is(err, errDeviceRevoked) {
			http.Error(w, "device revoked", http.StatusUnauthorized)
			return
		}
		if err != nil && c != nil {
			httperr.Internal(w, r, err)
			return
		}
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, c.UserID)
		ctx = context.WithValue(ctx, isAdminContextKey, c.IsAdmin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// VerifyAccessToken validates a raw access token string outside of
// the RequireAuth HTTP middleware flow, returning the authenticated
// user's id. Used by internal/realtime to authenticate a WebSocket
// connection's first message — browsers can't set the Authorization
// header on the handshake itself.
func (h *Handler) VerifyAccessToken(ctx context.Context, token string) (userID string, err error) {
	c, err := h.verifyAccessToken(ctx, token)
	if err != nil {
		return "", err
	}
	return c.UserID, nil
}

// verifyAccessToken checks the token's signature and expiry and then
// that its device still exists. The JWT alone would stay valid for its
// whole TTL (15 min by default) after a logout or a revocation; one
// primary-key lookup per request closes that window.
//
// On a database error it returns the parsed claims along with the
// error, so callers can answer 500 instead of a misleading 401.
func (h *Handler) verifyAccessToken(ctx context.Context, token string) (*claims, error) {
	c, err := parseAccessToken(h.jwtSecret, token)
	if err != nil {
		return nil, err
	}
	exists, err := h.store.deviceExists(ctx, c.DeviceID)
	if err != nil {
		return c, err
	}
	if !exists {
		return nil, errDeviceRevoked
	}
	return c, nil
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
