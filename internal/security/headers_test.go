package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveWithHeaders(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	handler := HeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHeadersMiddleware_CommonHeadersOnEveryResponse(t *testing.T) {
	for _, path := range []string{"/app/", "/admin/panel", "/tasks", "/health"} {
		rec := serveWithHeaders(t, path)
		want := map[string]string{
			"X-Content-Type-Options":     "nosniff",
			"Referrer-Policy":            "same-origin",
			"X-Frame-Options":            "DENY",
			"Cross-Origin-Opener-Policy": "same-origin",
			"Permissions-Policy":         "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
		}
		for name, value := range want {
			if got := rec.Header().Get(name); got != value {
				t.Errorf("%s: %s = %q, want %q", path, name, got, value)
			}
		}
	}
}

// The whole policy is compared, not substrings, so a duplicated or
// loosened directive shows up as a diff.
func TestHeadersMiddleware_AppPolicy(t *testing.T) {
	want := "default-src 'self'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; " +
		"img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; " +
		"worker-src 'none'; frame-src 'none'; manifest-src 'self'; object-src 'none'; " +
		"base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	for _, path := range []string{"/app", "/app/", "/app/notes/abc"} {
		if got := serveWithHeaders(t, path).Header().Get("Content-Security-Policy"); got != want {
			t.Errorf("%s CSP =\n%s\nwant\n%s", path, got, want)
		}
	}
}

// 'unsafe-inline' only for style attributes (Desktop's note formatting),
// never for scripts or <style> elements, and never 'unsafe-eval'.
func TestHeadersMiddleware_AppPolicyHasNoInlineScript(t *testing.T) {
	if strings.Contains(appCSP, "'unsafe-eval'") {
		t.Errorf("app CSP must not allow 'unsafe-eval'")
	}
	for _, directive := range strings.Split(appCSP, ";") {
		directive = strings.TrimSpace(directive)
		if strings.Contains(directive, "'unsafe-inline'") && !strings.HasPrefix(directive, "style-src-attr ") {
			t.Errorf("'unsafe-inline' outside style-src-attr: %q", directive)
		}
	}
}

// A path that only starts with "app" is not the SPA.
func TestHeadersMiddleware_LookalikePathIsNotApp(t *testing.T) {
	if csp := serveWithHeaders(t, "/application").Header().Get("Content-Security-Policy"); csp != apiCSP {
		t.Fatalf("/application got %q, want the API policy", csp)
	}
}

func TestHeadersMiddleware_PanelPolicy(t *testing.T) {
	for _, path := range []string{"/setup", "/admin/panel", "/admin/panel/login", "/admin/panel/users/1/delete", "/admin/static/logo.svg"} {
		if csp := serveWithHeaders(t, path).Header().Get("Content-Security-Policy"); csp != panelCSP {
			t.Errorf("%s got %q, want the panel policy", path, csp)
		}
	}
	want := "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; " +
		"frame-src 'none'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	if panelCSP != want {
		t.Errorf("panel CSP =\n%s\nwant\n%s", panelCSP, want)
	}
	// The panel's templates have no inline code anymore
	// (internal/auth/templates_test.go keeps it that way).
	if strings.Contains(panelCSP, "'unsafe-inline'") || strings.Contains(panelCSP, "'unsafe-eval'") {
		t.Errorf("panel CSP must not allow inline code: %s", panelCSP)
	}
}

// /admin/users is a JSON endpoint, not part of the panel: it gets the
// API policy, and so does any future route under /admin that isn't the
// panel.
func TestHeadersMiddleware_APIPolicy(t *testing.T) {
	for _, path := range []string{"/tasks", "/auth/login", "/ws", "/health", "/admin/users", "/admin", "/setup/", "/admin/panelx"} {
		if csp := serveWithHeaders(t, path).Header().Get("Content-Security-Policy"); csp != apiCSP {
			t.Errorf("%s got %q, want the API policy", path, csp)
		}
	}
}
