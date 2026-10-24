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

func TestAppIndexIsNeverCached(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/app/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("got Cache-Control %q for index.html, want %q — a stale cached index.html keeps pointing at old hashed assets forever after a redeploy", cc, "no-cache")
	}
}

func TestSetCacheHeaders(t *testing.T) {
	cases := []struct {
		reqPath string
		want    string
	}{
		{"", "no-cache"},
		{"index.html", "no-cache"},
		{"assets/index-abc123.js", "public, max-age=31536000, immutable"},
		{"logo.png", ""}, // no opinion — not hashed, not index.html
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		setCacheHeaders(rec, c.reqPath)
		if got := rec.Header().Get("Cache-Control"); got != c.want {
			t.Errorf("setCacheHeaders(%q): got Cache-Control %q, want %q", c.reqPath, got, c.want)
		}
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
