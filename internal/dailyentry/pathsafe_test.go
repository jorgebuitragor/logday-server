package dailyentry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/realtime"
)

// U-06: Desktop arma dailys/<año>/<mes>/ con slices fijos de la fecha, así
// que una fecha con separadores escribiría fuera de la carpeta de datos.
func TestPutRejectsPathLikeDate(t *testing.T) {
	h := NewHandler(newTestStore(t), nil, realtime.NewHub())
	r := chi.NewRouter()
	r.Put("/daily-entries/{date}", h.put)

	for _, path := range []string{
		"/daily-entries/..",
		"/daily-entries/..%2F.X..",
		"/daily-entries/..%5C..%5Cx",
		"/daily-entries/a%00b",
	} {
		req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		// The body is invalid too, so check the 400 comes from the date.
		if rec.Code != http.StatusBadRequest || !strings.HasPrefix(rec.Body.String(), "date ") {
			t.Errorf("%s: expected 400 about date, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}
