package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jorgebuitragor/logday-server/internal/security"
)

// csrfCookieTTL is fixed, unlike the panel session TTL (see
// Settings.PanelSessionTTL) — it only needs to survive from "render a
// form" to "submit it", not for the life of a whole session, so it isn't
// worth threading a live settings.Get() into every GET handler that just
// renders a form. Same numeric value (24h) the CSRF cookie always had,
// even back when it borrowed the (now configurable) session TTL for it.
const csrfCookieTTL = 24 * time.Hour

const (
	sessionCookieName = "logday_admin_session"
	csrfCookieName    = "logday_admin_csrf"
	csrfFormField     = "csrf_token"
)

type sessionClaims struct {
	UserID  string `json:"sub"`
	IsAdmin bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

// issuePanelSession signs a panel session valid for ttl — callers fetch
// ttl from internal/settings (Settings.PanelSessionTTL()) live at
// login/setup time instead of a fixed constant, so an operator can change
// it without restarting the server. The admin panel is a human clicking
// around occasionally from a browser, not an offline-capable sync client,
// so there's no multi-week session to bridge and no rotation/reuse-theft
// detection to build — see specs/panel-admin/design.md.
func issuePanelSession(secret []byte, userID string, isAdmin bool, ttl time.Duration) (string, error) {
	now := time.Now()
	c := sessionClaims{
		UserID:  userID,
		IsAdmin: isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return security.SignJWT(secret, c)
}

func parsePanelSession(secret []byte, tokenString string) (*sessionClaims, error) {
	c := &sessionClaims{}
	if err := security.ParseJWT(secret, tokenString, c); err != nil {
		return nil, err
	}
	return c, nil
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	//nolint:gosec // G124: Secure intentionally tracks r.TLS != nil, not a hardcoded true — self-hosted instances may run plain HTTP directly behind a LAN, see specs/panel-admin/design.md
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	//nolint:gosec // G124: same as setSessionCookie above
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// ensureCSRFCookie returns the current request's CSRF token, setting a
// fresh one on w if none is present yet. Called on every GET that
// renders a form (including /setup, which has no session to hang a
// token off yet) — the same value is echoed into a hidden form field by
// the template, and verifyCSRF checks the two match on POST
// (double-submit cookie pattern, no server-side storage needed since
// the token is only ever compared to itself).
func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) (string, error) {
	if c, err := r.Cookie(csrfCookieName); err == nil && c.Value != "" {
		return c.Value, nil
	}
	raw, _, err := security.GenerateOpaqueToken()
	if err != nil {
		return "", err
	}
	//nolint:gosec // G124: same as setSessionCookie above
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(csrfCookieTTL.Seconds()),
	})
	return raw, nil
}

// verifyCSRF reports whether r's csrf_token form field matches its CSRF
// cookie. Must be called after r.ParseForm().
func verifyCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	submitted := r.FormValue(csrfFormField)
	if submitted == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(submitted)) == 1
}

type panelSessionContextKey string

const panelUserIDContextKey panelSessionContextKey = "panel_user_id"

// requireAdminSession protects every /admin/panel/* route. Unlike
// RequireAuth/RequireAdmin (401/403 JSON, for the sync API), failure
// here redirects to the login page — the correct UX for an HTML
// surface. It deliberately doesn't trust the session cookie's IsAdmin
// claim: a live DB check on every request confirms the user is still an
// active admin right now, so a demote/soft-delete takes effect
// immediately instead of waiting out the cookie's 24h TTL.
func (h *Handler) requireAdminSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/admin/panel/login", http.StatusFound)
			return
		}

		claims, err := parsePanelSession(h.jwtSecret, cookie.Value)
		if err != nil {
			http.Redirect(w, r, "/admin/panel/login", http.StatusFound)
			return
		}

		u, err := h.store.getUserByID(r.Context(), claims.UserID)
		if err != nil || u.DeletedAt != nil || !u.IsAdmin {
			http.Redirect(w, r, "/admin/panel/login", http.StatusFound)
			return
		}

		ctx := context.WithValue(r.Context(), panelUserIDContextKey, u.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
