package security

import (
	"net/http"
	"strings"
)

// HeadersMiddleware sets defense-in-depth security headers on every
// response. The Content-Security-Policy depends on what the path
// serves:
//
//   - /app (logday-web, embedded): a strict policy. The SPA loads only
//     its own bundled scripts and styles, so no inline script, no
//     'unsafe-eval' and no <style> elements. What this buys is that an
//     injected script does not get to run in the page that holds the
//     user's session. It does not stop a script that already runs from
//     sending data out (images, navigation and window.open are still
//     possible); keeping the refresh token out of reach of scripts is
//     the job of the HttpOnly session cookie (specs/sesion-cookie/).
//   - The server-rendered panel (/setup, /admin/panel, /admin/static):
//     the templates still use inline <script>/<style> blocks and
//     onclick handlers, so script-src/style-src allow 'unsafe-inline'.
//     Everything else (framing, plugins, <base>, form targets,
//     connections) is locked to the same origin.
//   - Everything else (JSON API, including /admin/users, /health,
//     /ws): never rendered as a document, so the policy denies
//     everything.
//
// HSTS is deliberately not set here: TLS terminates at the reverse
// proxy (see specs/arquitectura-inicial), which is the layer that
// knows whether the instance is really served over HTTPS.
func HeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		// same-origin: a link inside a note doesn't leak /app/notes/<id>
		// to the site it points to, while same-origin requests keep
		// their Origin header (no-referrer would turn it into "null" on
		// same-origin POSTs, breaking any Origin-based CSRF check).
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Content-Security-Policy", contentSecurityPolicy(r.URL.Path))
		next.ServeHTTP(w, r)
	})
}

const (
	// connect-src 'self' covers fetch/XHR and the same-origin WebSocket
	// (/ws): CSP Level 3 matches ws:/wss: on the page's own host.
	//
	// img-src allows any https: image because notes are Markdown and may
	// embed external images; plain http: images are not allowed (an
	// https page would block them as mixed content anyway).
	//
	// style-src-attr allows inline style *attributes* only: Logday
	// Desktop's editor (Tiptap color, highlight and text-align) saves
	// formatting as <span style="..."> inside the note's Markdown, and
	// Web's preview would drop it otherwise. logday-web's sanitizer
	// keeps only color, background-color and text-align. <style>
	// elements stay blocked by style-src 'self'.
	appCSP = "default-src 'self'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; " +
		"img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; " +
		"worker-src 'none'; frame-src 'none'; manifest-src 'self'; object-src 'none'; " +
		"base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

	panelCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; " +
		"style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; " +
		"frame-src 'none'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

	apiCSP = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
)

func contentSecurityPolicy(path string) string {
	switch {
	case path == "/app" || strings.HasPrefix(path, "/app/"):
		return appCSP
	case path == "/setup" || path == "/admin/panel" ||
		strings.HasPrefix(path, "/admin/panel/") || strings.HasPrefix(path, "/admin/static/"):
		return panelCSP
	default:
		return apiCSP
	}
}
