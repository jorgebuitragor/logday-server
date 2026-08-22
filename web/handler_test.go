package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestRouter() *chi.Mux {
	r := chi.NewRouter()
	Routes(r)
	return r
}

func TestAppRedirectsToTrailingSlash(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusMovedPermanently)
	}
	if loc := rec.Header().Get("Location"); loc != "/app/" {
		t.Fatalf("got Location %q, want /app/", loc)
	}
}

func TestAppRootServesIndex(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/app/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<!doctype html>") {
		t.Fatalf("expected index.html content, got %q", body)
	}
}

func TestAppUnknownClientRouteFallsBackToIndex(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/app/tasks/some-id", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 (SPA fallback)", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<!doctype html>") {
		t.Fatalf("expected index.html content as fallback, got %q", body)
	}
}

func TestAppDoesNotShadowAPIRoutes(t *testing.T) {
	r := newTestRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("expected /health to reach its own handler, got status %d body %q", rec.Code, rec.Body.String())
	}
}
