package security

import (
	"net/http"
	"regexp"
	"strings"
)

// HeadersMiddleware sets defense-in-depth security headers on every
// response. The Content-Security-Policy depends on what the path
// serves:
//
//   - /app (logday-web, embedded): a strict policy. The SPA loads only
//     its own bundled scripts and styles, so no 'unsafe-inline' and no
//     'unsafe-eval'. This is what keeps an injected script from running
//     in the page that holds the user's session (see
//     specs/cabeceras-seguridad/design.md).
//   - /admin and /setup (server-rendered panel): the templates still
//     use inline <script>/<style> blocks and onclick handlers, so
//     script-src/style-src allow 'unsafe-inline'. Everything else
//     (framing, plugins, <base>, form targets, connections) is locked
//     to the same origin.
//   - Everything else (JSON API, /health, /ws): never rendered as a
//     document, so the policy denies everything.
//
// HSTS is deliberately not set here: TLS terminates at the reverse
// proxy (see specs/arquitectura-inicial), which is the layer that
// knows whether the instance is really served over HTTPS.
func HeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		// no-referrer: a link inside a note would otherwise leak
		// /app/notes/<id> to whatever site it points to.
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Content-Security-Policy", contentSecurityPolicy(r))
		next.ServeHTTP(w, r)
	})
}

const (
	// img-src allows any https: image because notes are Markdown and may
	// embed external images. That also leaves images as a possible
	// exfiltration channel; connect-src is still restricted to the same
	// origin, so fetch/XHR/WebSocket cannot reach another host.
	//
	// style-src-attr allows inline style *attributes* only: Logday
	// Desktop's editor (Tiptap color, highlight and text-align) saves
	// formatting as <span style="..."> inside the note's Markdown, and
	// Web's preview would drop it otherwise. A style attribute cannot
	// run code or use selectors to read the page; <style> elements stay
	// blocked by style-src 'self'.
	appCSPBase = "default-src 'self'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; " +
		"img-src 'self' data: blob: https:; font-src 'self' data:; " +
		"worker-src 'self' blob:; manifest-src 'self'; object-src 'none'; " +
		"base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

	panelCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; " +
		"style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; " +
		"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

	apiCSP = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
)

// validHost matches what a Host header can legitimately contain
// (hostname or IP, optional port, bracketed IPv6). Anything else is
// left out of the policy instead of being echoed into a header.
var validHost = regexp.MustCompile(`^[A-Za-z0-9.\-]+(:[0-9]{1,5})?$|^\[[0-9A-Fa-f:.]+\](:[0-9]{1,5})?$`)

func contentSecurityPolicy(r *http.Request) string {
	path := r.URL.Path
	switch {
	case path == "/app" || strings.HasPrefix(path, "/app/"):
		return appCSPBase + "; connect-src " + appConnectSrc(r.Host)
	case path == "/setup" || path == "/admin" || strings.HasPrefix(path, "/admin/"):
		return panelCSP
	default:
		return apiCSP
	}
}

// appConnectSrc lists the same-origin WebSocket URLs explicitly: CSP
// Level 3 says 'self' covers ws:/wss: on the same host, but older
// WebKit builds only matched the page's own scheme, which would block
// the realtime channel (/ws). Both schemes are listed because, behind a
// TLS-terminating proxy, the server cannot tell which one the browser
// is using.
func appConnectSrc(host string) string {
	if !validHost.MatchString(host) {
		return "'self'"
	}
	return "'self' ws://" + host + " wss://" + host
}
