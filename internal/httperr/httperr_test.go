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
	if !strings.Contains(logged, "POST /notes/abc/content: storing merged content: database is locked") {
		t.Fatalf("log = %q, missing method, path or error", logged)
	}
	if strings.Contains(logged, "secreto") {
		t.Fatalf("log = %q, leaked the query string", logged)
	}
}
