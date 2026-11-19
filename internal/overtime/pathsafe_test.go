package overtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/realtime"
)

// U-06: Desktop guarda cada mes en overtime/<año>/<mes>/, con el año y el
// mes cortados de fecha (y de yearMonth) con slices fijos.

func newPathTestRouter(t *testing.T) http.Handler {
	t.Helper()
	h := NewHandler(newTestStore(t), nil, realtime.NewHub())
	r := chi.NewRouter()
	r.Post("/overtime-entries", h.createEntry)
	r.Patch("/overtime-entries/{id}", h.patchEntry)
	r.Patch("/overtime-month-meta/{yearMonth}", h.patchMonthMeta)
	return r
}

func send(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func entryBody(id, fecha string) string {
	return `{"id":"` + id + `","fecha":"` + fecha + `","updated_at":"2026-11-19T10:00:00Z"}`
}

func expect400(t *testing.T, rec *httptest.ResponseRecorder, field string) {
	t.Helper()
	if rec.Code != http.StatusBadRequest || !strings.HasPrefix(rec.Body.String(), field+" ") {
		t.Fatalf("expected 400 about %s, got %d: %s", field, rec.Code, rec.Body.String())
	}
}

func TestOvertimeRejectsPathLikeFields(t *testing.T) {
	r := newPathTestRouter(t)
	expect400(t, send(t, r, http.MethodPost, "/overtime-entries", entryBody("e1", "../.X..")), "fecha")
	expect400(t, send(t, r, http.MethodPost, "/overtime-entries", entryBody("../e1", "2026-11-19")), "id")

	if rec := send(t, r, http.MethodPost, "/overtime-entries", entryBody("e1", "2026-11-19")); rec.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	expect400(t, send(t, r, http.MethodPatch, "/overtime-entries/e1", `{"fecha":"20/11/2026","updated_at":"2026-11-19T10:01:00Z"}`), "fecha")
	expect400(t, send(t, r, http.MethodPatch, "/overtime-month-meta/..%2F..", `{"updated_at":"2026-11-19T10:01:00Z"}`), "yearMonth")

	if rec := send(t, r, http.MethodPatch, "/overtime-month-meta/2026-11", `{"colaborador":"x","updated_at":"2026-11-19T10:01:00Z"}`); rec.Code != http.StatusOK {
		t.Fatalf("month meta: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
