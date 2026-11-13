package compat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestVersionHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	VersionHandler(rec, httptest.NewRequest(http.MethodGet, "/version", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var body versionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body.Protocol != Protocol || body.MinClientProtocol != MinClientProtocol || body.ServerVersion != ServerVersion {
		t.Fatalf("body = %+v", body)
	}
	if body.MinClientProtocol > body.Protocol {
		t.Fatalf("min_client_protocol (%d) can't exceed protocol (%d)", body.MinClientProtocol, body.Protocol)
	}
}

func TestRequireClientProtocol(t *testing.T) {
	cases := []struct {
		name    string
		min     int
		header  string
		want    int
		wantMin string
	}{
		{"sin cabecera cuenta como 1, mínimo 1", 1, "", http.StatusOK, ""},
		{"sin cabecera cuenta como 1, mínimo 2", 2, "", http.StatusUpgradeRequired, "2"},
		{"igual al mínimo", 2, "2", http.StatusOK, ""},
		{"más nuevo que el mínimo", 1, "3", http.StatusOK, ""},
		{"más viejo que el mínimo", 3, "2", http.StatusUpgradeRequired, "3"},
		{"no es un número", 1, "abc", http.StatusBadRequest, ""},
		{"cero", 1, "0", http.StatusBadRequest, ""},
		{"negativo", 1, "-1", http.StatusBadRequest, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/sync/changes", nil)
			if tc.header != "" {
				req.Header.Set(Header, tc.header)
			}
			rec := httptest.NewRecorder()
			RequireClientProtocol(tc.min)(okHandler()).ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if got := rec.Header().Get(MinHeader); got != tc.wantMin {
				t.Fatalf("%s = %q, want %q", MinHeader, got, tc.wantMin)
			}
		})
	}
}

func TestRequireClientProtocol_ExemptPathAlwaysPasses(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set(Header, "1")
	rec := httptest.NewRecorder()
	RequireClientProtocol(2, "/auth/logout")(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
