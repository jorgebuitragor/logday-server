package note

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/realtime"
)

// U-06: Desktop guarda cada nota en notes/<folder>/<id>.md.

func newPathTestRouter(t *testing.T) http.Handler {
	t.Helper()
	h := NewHandler(newTestStore(t), nil, realtime.NewHub())
	r := chi.NewRouter()
	r.Post("/notes", h.create)
	r.Patch("/notes/{id}", h.patch)
	return r
}

func send(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func noteBody(id, folder string) string {
	return `{"id":` + quote(id) + `,"title":"t","folder":` + quote(folder) +
		`,"created":"2026-11-19T10:00:00.000Z","updated_at":"2026-11-19T10:00:00Z"}`
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"` }

func TestCreateNoteRejectsPathLikeIDAndFolder(t *testing.T) {
	cases := []struct{ name, id, folder, wantField string }{
		{"folder con traversal", "n1", "../../x", "folder"},
		{"folder absoluto", "n1", "/etc", "folder"},
		{"folder con barra invertida", "n1", `a\..\b`, "folder"},
		{"id con barra", "../n1", "", "id"},
		{"id dos puntos", "..", "", "id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := send(t, newPathTestRouter(t), http.MethodPost, "/notes", noteBody(tc.id, tc.folder))
			if rec.Code != http.StatusBadRequest || !strings.HasPrefix(rec.Body.String(), tc.wantField+" ") {
				t.Fatalf("expected 400 about %s, got %d: %s", tc.wantField, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateNoteAcceptsSubfoldersAndLegacyIDs(t *testing.T) {
	for _, folder := range []string{"", "trabajo", "trabajo/clientes 2026"} {
		rec := send(t, newPathTestRouter(t), http.MethodPost, "/notes", noteBody("nota-legada-1", folder))
		if rec.Code != http.StatusOK {
			t.Fatalf("folder %q: expected 200, got %d: %s", folder, rec.Code, rec.Body.String())
		}
	}
}

func TestPatchNoteRejectsPathLikeFolder(t *testing.T) {
	r := newPathTestRouter(t)
	if rec := send(t, r, http.MethodPost, "/notes", noteBody("n1", "a")); rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec := send(t, r, http.MethodPatch, "/notes/n1", `{"folder":"a/../../b","updated_at":"2026-11-19T10:01:00Z"}`)
	if rec.Code != http.StatusBadRequest || !strings.HasPrefix(rec.Body.String(), "folder ") {
		t.Fatalf("expected 400 about folder, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = send(t, r, http.MethodPatch, "/notes/n1", `{"folder":"a/b","updated_at":"2026-11-19T10:02:00Z"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid move: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
