package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveWithHeaders(t *testing.T, path, host string) *httptest.ResponseRecorder {
	t.Helper()
	handler := HeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if host != "" {
		req.Host = host
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHeadersMiddleware_CommonHeadersOnEveryResponse(t *testing.T) {
	for _, path := range []string{"/app/", "/admin/panel", "/tasks", "/health"} {
		rec := serveWithHeaders(t, path, "")
		want := map[string]string{
			"X-Content-Type-Options":     "nosniff",
			"Referrer-Policy":            "no-referrer",
			"X-Frame-Options":            "DENY",
			"Cross-Origin-Opener-Policy": "same-origin",
		}
		for name, value := range want {
			if got := rec.Header().Get(name); got != value {
				t.Errorf("%s: %s = %q, want %q", path, name, got, value)
			}
		}
		if rec.Header().Get("Content-Security-Policy") == "" {
			t.Errorf("%s: missing Content-Security-Policy", path)
		}
	}
}

func TestHeadersMiddleware_AppPolicyIsStrict(t *testing.T) {
	csp := serveWithHeaders(t, "/app/notes/abc", "logday.example.com").Header().Get("Content-Security-Policy")

	for _, directive := range []string{
		"script-src 'self';",
		"style-src 'self';",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"connect-src 'self' ws://logday.example.com wss://logday.example.com",
	} {
		if !strings.Contains(csp, directive) {
			t.Errorf("app CSP missing %q:\n%s", directive, csp)
		}
	}
	if strings.Contains(csp, "'unsafe-eval'") {
		t.Errorf("app CSP must not allow 'unsafe-eval':\n%s", csp)
	}
	// 'unsafe-inline' is allowed only for style attributes (Desktop's
	// note formatting), never for scripts or <style> elements.
	for _, directive := range strings.Split(csp, ";") {
		directive = strings.TrimSpace(directive)
		if strings.Contains(directive, "'unsafe-inline'") && !strings.HasPrefix(directive, "style-src-attr ") {
			t.Errorf("'unsafe-inline' outside style-src-attr: %q", directive)
		}
	}
	if !strings.Contains(csp, "style-src-attr 'unsafe-inline'") {
		t.Errorf("app CSP should allow inline style attributes:\n%s", csp)
	}
}

func TestHeadersMiddleware_AppRedirectPathGetsAppPolicy(t *testing.T) {
	csp := serveWithHeaders(t, "/app", "").Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self';") {
		t.Fatalf("/app should get the app policy, got:\n%s", csp)
	}
}

// A path that only starts with "app" is not the SPA: it must not get
// the app policy (and its connect-src) by accident.
func TestHeadersMiddleware_LookalikePathIsNotApp(t *testing.T) {
	csp := serveWithHeaders(t, "/application", "").Header().Get("Content-Security-Policy")
	if csp != apiCSP {
		t.Fatalf("/application got %q, want the API policy", csp)
	}
}

func TestHeadersMiddleware_PanelPolicyAllowsInlineButLocksTheRest(t *testing.T) {
	for _, path := range []string{"/setup", "/admin/panel/login", "/admin/static/logo.svg"} {
		csp := serveWithHeaders(t, path, "").Header().Get("Content-Security-Policy")
		if csp != panelCSP {
			t.Errorf("%s got %q, want the panel policy", path, csp)
		}
	}
	for _, directive := range []string{"connect-src 'self'", "frame-ancestors 'none'", "form-action 'self'", "object-src 'none'"} {
		if !strings.Contains(panelCSP, directive) {
			t.Errorf("panel CSP missing %q", directive)
		}
	}
}

func TestHeadersMiddleware_APIPolicyDeniesEverything(t *testing.T) {
	for _, path := range []string{"/tasks", "/auth/login", "/ws", "/health"} {
		if csp := serveWithHeaders(t, path, "").Header().Get("Content-Security-Policy"); csp != apiCSP {
			t.Errorf("%s got %q, want the API policy", path, csp)
		}
	}
}

// The Host header is client-controlled: anything that doesn't look
// like a host must not be copied into the policy.
func TestHeadersMiddleware_InvalidHostIsNotEchoed(t *testing.T) {
	for _, host := range []string{"evil.example; script-src *", "a b", "host\"x"} {
		csp := serveWithHeaders(t, "/app/", host).Header().Get("Content-Security-Policy")
		if !strings.HasSuffix(csp, "connect-src 'self'") {
			t.Errorf("host %q leaked into the policy:\n%s", host, csp)
		}
	}
}

func TestHeadersMiddleware_ValidHostsWithPortAndIPv6(t *testing.T) {
	for _, host := range []string{"localhost:8080", "192.168.1.10:8080", "[::1]:8080", "logday.jorgebuitrago.com"} {
		csp := serveWithHeaders(t, "/app/", host).Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "wss://"+host) {
			t.Errorf("host %q missing from connect-src:\n%s", host, csp)
		}
	}
}
