package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
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
// (security.CORSMiddleware only allows Authorization and Content-Type
// and never sends Access-Control-Allow-Credentials — keep it that way,
// see security/cors_test.go). SameSite=Strict blocks the cookie on
// cross-site requests anyway.
const (
	sessionModeHeader = "X-Logday-Session"
	sessionModeCookie = "cookie"

	// Over HTTPS the cookie uses the __Host- prefix: the browser then
	// only accepts it if it is Secure, has Path=/ and no Domain, so a
	// sibling subdomain (another service on the same parent domain)
	// can't plant a cookie with the same name that would shadow ours
	// ("cookie tossing"). The prefix forces Path=/, which sends the
	// cookie to every endpoint, but the server only reads it on
	// /auth/* and only with the header above.
	secureRefreshCookieName = "__Host-logday_refresh"
	// Over plain HTTP (a LAN instance) the prefix isn't allowed, so the
	// cookie is scoped to /auth instead.
	plainRefreshCookieName = "logday_refresh"
	plainRefreshCookiePath = "/auth"
)

func cookieMode(r *http.Request) bool {
	return r.Header.Get(sessionModeHeader) == sessionModeCookie
}

// secureRequest reports whether the browser reached the server over
// HTTPS. TLS normally terminates at the reverse proxy (see
// specs/arquitectura-inicial), so r.TLS is nil there and the proxy's
// X-Forwarded-Proto is what says it. Trusting that header only decides
// the cookie's Secure flag: a client that lies about it can only break
// its own cookie, not weaken anyone else's. COOKIE_SECURE=always skips
// the detection, for a proxy chain that doesn't forward the scheme
// faithfully (e.g. a CDN in "flexible" TLS mode in front of the proxy).
func secureRequest(r *http.Request) bool {
	if os.Getenv("COOKIE_SECURE") == "always" {
		return true
	}
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func refreshCookieNameAndPath(r *http.Request) (string, string) {
	if secureRequest(r) {
		return secureRefreshCookieName, "/"
	}
	return plainRefreshCookieName, plainRefreshCookiePath
}

func setRefreshCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	name, path := refreshCookieNameAndPath(r)
	//nolint:gosec // G124: Secure follows secureRequest(r), not a hardcoded true — a self-hosted instance may be served over plain HTTP on a LAN, where a Secure cookie would never be stored (same reasoning as setSessionCookie in panel_session.go)
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     path,
		HttpOnly: true,
		Secure:   secureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearRefreshCookie(w http.ResponseWriter, r *http.Request) {
	name, path := refreshCookieNameAndPath(r)
	//nolint:gosec // G124: same as setRefreshCookie above
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		HttpOnly: true,
		Secure:   secureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// refreshCookieValue returns the session cookie's value, or "" when
// there is none — or more than one: two cookies with our name means
// one was planted (a different path or a parent domain), and picking
// either could log the user into someone else's account.
func refreshCookieValue(r *http.Request) string {
	name, _ := refreshCookieNameAndPath(r)
	value, count := "", 0
	for _, c := range r.Cookies() {
		if c.Name == name {
			value = c.Value
			count++
		}
	}
	if count != 1 {
		return ""
	}
	return value
}

// refreshTokenFromRequest decodes an optional {"refresh_token": "..."}
// body and, in cookie mode, prefers the cookie. The body only counts
// in cookie mode when there is no cookie yet: that is how a browser
// that still has a refresh token from before cookie mode (in
// localStorage) moves it into the cookie on its first refresh. Once
// the cookie exists it wins, so a second tab that still holds the old
// localStorage token can't present it again after the first tab
// migrated (which would look like reuse and revoke the device). An
// empty body is valid in cookie mode, where the cookie is the normal
// carrier.
func refreshTokenFromRequest(r *http.Request) (string, error) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && (!errors.Is(err, io.EOF) || !cookieMode(r)) {
		return "", err
	}
	if !cookieMode(r) {
		return req.RefreshToken, nil
	}
	if c := refreshCookieValue(r); c != "" {
		return c, nil
	}
	return req.RefreshToken, nil
}

// logout ends the session that the presented refresh token belongs to:
// it revokes that device (same effect as removing it from the devices
// list) and, in cookie mode, clears the cookie — the only way to drop
// an HttpOnly cookie, since page scripts can't touch it. A token that
// was already rotated (a refresh in flight when the user logged out)
// still revokes its device. Always 204, also for an unknown or
// already-revoked token, so logging out twice or after the session
// expired is not an error.
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
		hash := security.HashOpaqueToken(token)
		if d, err := h.store.getDeviceByRefreshTokenHash(r.Context(), hash); err == nil {
			_ = h.store.deleteDevice(r.Context(), d.ID)
		} else {
			_ = h.store.revokeDeviceByUsedTokenHash(r.Context(), hash)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// revokeCookieSessionOnLogin: logging in again in cookie mode while
// the browser still holds a working session cookie (the page lost its
// hint in localStorage, or restoring failed on a network error) would
// otherwise leave the old device alive next to the new one. The
// presented cookie proves possession, so its device is revoked.
func (h *Handler) revokeCookieSessionOnLogin(r *http.Request) {
	if !cookieMode(r) {
		return
	}
	if c := refreshCookieValue(r); c != "" {
		if d, err := h.store.getDeviceByRefreshTokenHash(r.Context(), security.HashOpaqueToken(c)); err == nil {
			_ = h.store.deleteDevice(r.Context(), d.ID)
		}
	}
}
