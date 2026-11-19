package task

import (
	"net/http"
	"strings"
	"testing"
)

// U-06: Desktop guarda cada tarea en projects/<project>/<id>.md.

func taskBody(id, project string) string {
	return `{"id":"` + id + `","title":"t","status":"todo","project":"` + project +
		`","created":"2026-11-19T10:00:00.000Z","updated_at":"2026-11-19T10:00:00Z"}`
}

func TestCreateTaskRejectsPathLikeIDAndProject(t *testing.T) {
	cases := []struct{ name, id, project, wantField string }{
		{"project con traversal", "t1", "../../x", "project"},
		{"project con barra", "t1", "a/b", "project"},
		{"project dos puntos", "t1", "..", "project"},
		{"id con barra", "a/../../t1", "p", "id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, newTestRouter(t), http.MethodPost, "/tasks", taskBody(tc.id, tc.project))
			if rec.Code != http.StatusBadRequest || !strings.HasPrefix(rec.Body.String(), tc.wantField+" ") {
				t.Fatalf("expected 400 about %s, got %d: %s", tc.wantField, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateTaskAcceptsUsualProjects(t *testing.T) {
	for _, project := range []string{"", "general", "Fábrica de software"} {
		rec := doJSON(t, newTestRouter(t), http.MethodPost, "/tasks", taskBody("task-legada", project))
		if rec.Code != http.StatusOK {
			t.Fatalf("project %q: expected 200, got %d: %s", project, rec.Code, rec.Body.String())
		}
	}
}

func TestPatchTaskRejectsPathLikeProject(t *testing.T) {
	r := newTestRouter(t)
	if rec := doJSON(t, r, http.MethodPost, "/tasks", taskBody("t1", "p")); rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec := doJSON(t, r, http.MethodPatch, "/tasks/t1", `{"project":"../x","updated_at":"2026-11-19T10:01:00Z"}`)
	if rec.Code != http.StatusBadRequest || !strings.HasPrefix(rec.Body.String(), "project ") {
		t.Fatalf("expected 400 about project, got %d: %s", rec.Code, rec.Body.String())
	}
}
