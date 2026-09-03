package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newCORSTestHandler() http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return CORSMiddleware([]string{"https://allowed.example"})(next)
}

func TestCORSMiddleware_EmptyAllowlistIsNoop(t *testing.T) {
	handler := CORSMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	req.Header.Set("Origin", "https://anything.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin header, got %q", got)
	}
}

func TestCORSMiddleware_DisallowedOriginGetsNoHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	req.Header.Set("Origin", "https://not-allowed.example")
	rec := httptest.NewRecorder()
	newCORSTestHandler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin header, got %q", got)
	}
}

func TestCORSMiddleware_AllowedOriginReflected(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	req.Header.Set("Origin", "https://allowed.example")
	rec := httptest.NewRecorder()
	newCORSTestHandler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://allowed.example", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("expected no Access-Control-Allow-Credentials header, got %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (request should reach next handler)", rec.Code, http.StatusOK)
	}
}

func TestCORSMiddleware_PreflightHandledDirectly(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/tasks", nil)
	req.Header.Set("Origin", "https://allowed.example")
	rec := httptest.NewRecorder()
	newCORSTestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://allowed.example", got)
	}
	// PUT incluido a propósito: /daily-entries/{date} y otras rutas de
	// clave natural lo usan (internal/dailyentry/handlers.go) — un
	// preflight sin PUT bloquearía esas requests del lado navegador
	// aunque la ruta real sí lo soporte.
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "PUT") {
		t.Fatalf("Access-Control-Allow-Methods = %q, want it to include PUT", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("expected Access-Control-Allow-Headers to be set")
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got == "" {
		t.Fatal("expected Access-Control-Max-Age to be set")
	}
}
