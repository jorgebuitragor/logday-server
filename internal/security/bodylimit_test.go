package security

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimitFor(t *testing.T) {
	const def = 3 << 20
	cases := map[string]int64{
		"/auth/login":                    AuthBodyLimit,
		"/auth/refresh":                  AuthBodyLimit,
		"/setup":                         PanelBodyLimit,
		"/admin/panel/login":             PanelBodyLimit,
		"/admin/panel/settings":          PanelBodyLimit,
		"/notes":                         def,
		"/admin/users":                   def,
		"/daily-entries/2026-11-27":      def,
		"/authors-is-not-an-auth-prefix": def,
	}
	for path, want := range cases {
		if got := BodyLimitFor(path, def); got != want {
			t.Errorf("%s: got %d, want %d", path, got, want)
		}
	}
}

func TestBodyLimitMiddleware(t *testing.T) {
	var readErr error
	h := BodyLimitMiddleware(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("Content-Length over the limit gets 413 without reaching the handler", func(t *testing.T) {
		readErr = errors.New("not called")
		r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(strings.Repeat("x", int(AuthBodyLimit)+1)))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413", w.Code)
		}
		if readErr == nil || readErr.Error() != "not called" {
			t.Fatal("handler should not run")
		}
	})

	t.Run("unknown length over the limit fails the read", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/notes", io.NopCloser(strings.NewReader(strings.Repeat("x", 2048))))
		r.ContentLength = -1
		h.ServeHTTP(httptest.NewRecorder(), r)
		var maxErr *http.MaxBytesError
		if !errors.As(readErr, &maxErr) {
			t.Fatalf("read error = %v, want *http.MaxBytesError", readErr)
		}
	})

	t.Run("within the limit passes", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/notes", strings.NewReader(strings.Repeat("x", 1024)))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent || readErr != nil {
			t.Fatalf("status = %d, read error = %v", w.Code, readErr)
		}
	})
}
