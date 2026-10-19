package httperr

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestInternalLogsAndHidesError(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/notes/abc/content?token=secreto", nil)
	Internal(w, r, errors.New("storing merged content: database is locked"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if body := strings.TrimSpace(w.Body.String()); body != "internal error" {
		t.Fatalf("body = %q, want the generic message", body)
	}
	logged := buf.String()
	if !strings.Contains(logged, `"POST" "/notes/abc/content": storing merged content: database is locked`) {
		t.Fatalf("log = %q, missing method, path or error", logged)
	}
	if strings.Contains(logged, "secreto") {
		t.Fatalf("log = %q, leaked the query string", logged)
	}
}

// Un %0A en la ruta llega decodificado como salto de línea en
// r.URL.Path: sin citarlo, el resto de la ruta se loguearía como una
// línea nueva con el contenido que eligiera el cliente.
func TestInternalEscapesNewlinesInPath(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/notes/x%0A2026/01/01%2000:00:00%20admin%20login%20ok", nil)
	Internal(w, r, errors.New("boom"))

	logged := strings.TrimSuffix(buf.String(), "\n")
	if strings.Contains(logged, "\n") {
		t.Fatalf("log = %q, the path injected a new log line", logged)
	}
	if !strings.Contains(logged, `\n2026/01/01 00:00:00 admin login ok`) {
		t.Fatalf("log = %q, want the newline escaped inside the quoted path", logged)
	}
}
