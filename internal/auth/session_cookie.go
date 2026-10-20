package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/security"
)

// Cookie session mode (specs/sesion-cookie/). A browser client served
// same-origin (logday-web embedded under /app) opts in by sending
// X-Logday-Session: cookie on /auth/login, /auth/refresh and
// /auth/logout. In that mode the refresh token travels only in an
// HttpOnly cookie — never in the JSON body — so no script running in
// the page can read it. Clients that don't send the header (Desktop,
// Mobile, standalone logday-web on another origin) keep the original
// body-based contract unchanged.
//
// The custom header is also the CSRF guard: the cookie is only read
// when the header is present, and a cross-site page can't add a custom
// header without a CORS preflight that the server never approves
// (security.CORSMiddleware only allows Authorization and Content-Type).
// SameSite=Strict blocks the cookie on cross-site requests anyway.
const (
	sessionModeHeader = "X-Logday-Session"
	sessionModeCookie = "cookie"
	refreshCookieName = "logday_refresh"
	// Path=/auth: the cookie only goes to /auth/refresh and
	// /auth/logout (and /auth/login, harmlessly), never to the data API.
	refreshCookiePath = "/auth"
)

func cookieMode(r *http.Request) bool {
	return r.Header.Get(sessionModeHeader) == sessionModeCookie
}

// secureRequest reports whether the browser reached the server over
// HTTPS. TLS normally terminates at the reverse proxy (see
// specs/arquitectura-inicial), so r.TLS is nil there and the proxy's
// X-Forwarded-Proto is what says it. Trusting that header only decides
// the cookie's Secure flag: a client that lies about it can only break
// its own cookie, not weaken anyone else's.
func secureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setRefreshCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	//nolint:gosec // G124: Secure follows secureRequest(r), not a hardcoded true — a self-hosted instance may be served over plain HTTP on a LAN, where a Secure cookie would never be stored (same reasoning as setSessionCookie in panel_session.go)
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		HttpOnly: true,
		Secure:   secureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearRefreshCookie(w http.ResponseWriter, r *http.Request) {
	//nolint:gosec // G124: same as setRefreshCookie above
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		HttpOnly: true,
		Secure:   secureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// refreshTokenFromRequest decodes an optional {"refresh_token": "..."}
// body and falls back to the cookie when the request is in cookie
// mode. The body wins when both are present: that is how a browser
// that still has a refresh token from before cookie mode (in
// localStorage) moves it into the cookie on its first refresh. An
// empty body is valid in cookie mode, where the cookie is the normal
// carrier.
func refreshTokenFromRequest(r *http.Request) (string, error) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && (!errors.Is(err, io.EOF) || !cookieMode(r)) {
		return "", err
	}
	if req.RefreshToken != "" || !cookieMode(r) {
		return req.RefreshToken, nil
	}
	if c, err := r.Cookie(refreshCookieName); err == nil {
		return c.Value, nil
	}
	return "", nil
}

// logout ends the session that the presented refresh token belongs to:
// it revokes that device (same effect as removing it from the devices
// list) and, in cookie mode, clears the cookie — the only way to drop
// an HttpOnly cookie, since page scripts can't touch it. Always 204,
// also for an unknown or already-revoked token, so logging out twice
// or after the session expired is not an error.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	token, err := refreshTokenFromRequest(r)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if cookieMode(r) {
		clearRefreshCookie(w, r)
	}
	if token != "" {
		if d, err := h.store.getDeviceByRefreshTokenHash(r.Context(), security.HashOpaqueToken(token)); err == nil {
			_ = h.store.deleteDevice(r.Context(), d.ID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
