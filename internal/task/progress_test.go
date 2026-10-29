package task

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/db"
	"github.com/jorgebuitragor/logday-server/internal/realtime"
)

// Tests de Task.progress (specs/esquema-datos/, "Progreso de tareas").

func intPtr(v int) *int { return &v }

func progressString(p *int) string {
	if p == nil {
		return "<nil>"
	}
	b, _ := json.Marshal(*p)
	return string(b)
}

func TestUpsertTaskStoresProgress(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := sampleTask("user-1", time.Now())
	in.Progress = intPtr(40)
	if _, err := s.upsertTask(ctx, in, true); err != nil {
		t.Fatalf("upsertTask: %v", err)
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Progress == nil || *tasks[0].Progress != 40 {
		t.Fatalf("expected progress 40, got %+v", tasks)
	}
}

func TestUpsertTaskWithoutProgressStoresNull(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertTask(ctx, sampleTask("user-1", time.Now()), false); err != nil {
		t.Fatalf("upsertTask: %v", err)
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Progress != nil {
		t.Fatalf("expected progress nil, got %s", progressString(tasks[0].Progress))
	}
}

// Un cliente publicado antes de que existiera el campo nunca manda
// progress: su POST sobre una tarea existente no debe borrarlo.
func TestUpsertTaskWithoutProgressKeepsExistingValue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	in := sampleTask("user-1", base)
	in.Progress = intPtr(70)
	if _, err := s.upsertTask(ctx, in, true); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	legacy := sampleTask("user-1", base.Add(time.Minute))
	legacy.Title = "edited by an old client"
	stored, err := s.upsertTask(ctx, legacy, false)
	if err != nil {
		t.Fatalf("legacy upsertTask: %v", err)
	}
	if stored.Progress == nil || *stored.Progress != 70 {
		t.Fatalf("expected the response to keep progress 70, got %s", progressString(stored.Progress))
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if tasks[0].Title != "edited by an old client" || tasks[0].Progress == nil || *tasks[0].Progress != 70 {
		t.Fatalf("expected new title and progress 70, got %+v progress=%s", tasks[0], progressString(tasks[0].Progress))
	}

	// Con la clave presente (aunque sea null), el POST sí reemplaza.
	cleared := sampleTask("user-1", base.Add(2*time.Minute))
	stored, err = s.upsertTask(ctx, cleared, true)
	if err != nil {
		t.Fatalf("clearing upsertTask: %v", err)
	}
	if stored.Progress != nil {
		t.Fatalf("expected an explicit null to clear progress, got %s", progressString(stored.Progress))
	}
}

func TestPatchTaskSetsProgress(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertTask(ctx, sampleTask("user-1", base), true); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	patch := Patch{Progress: db.Field[*int]{Set: true, Value: intPtr(25)}}
	stored, changed, err := s.patchTask(ctx, "task-1", "user-1", patch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchTask: %v", err)
	}
	if !changed || stored.Progress == nil || *stored.Progress != 25 {
		t.Fatalf("expected progress 25, got changed=%v progress=%s", changed, progressString(stored.Progress))
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if tasks[0].Progress == nil || *tasks[0].Progress != 25 {
		t.Fatalf("expected stored progress 25, got %s", progressString(tasks[0].Progress))
	}
}

func TestPatchTaskExplicitNullClearsProgress(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	in := sampleTask("user-1", base)
	in.Progress = intPtr(60)
	if _, err := s.upsertTask(ctx, in, true); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	patch := Patch{Progress: db.Field[*int]{Set: true, Value: nil}}
	stored, changed, err := s.patchTask(ctx, "task-1", "user-1", patch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchTask: %v", err)
	}
	if !changed || stored.Progress != nil {
		t.Fatalf("expected progress cleared, got changed=%v progress=%s", changed, progressString(stored.Progress))
	}
}

func TestPatchTaskWithoutProgressLeavesItAlone(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	in := sampleTask("user-1", base)
	in.Progress = intPtr(60)
	if _, err := s.upsertTask(ctx, in, true); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	patch := Patch{Title: db.Field[string]{Set: true, Value: "only the title"}}
	stored, changed, err := s.patchTask(ctx, "task-1", "user-1", patch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchTask: %v", err)
	}
	if !changed || stored.Progress == nil || *stored.Progress != 60 {
		t.Fatalf("expected progress 60 untouched, got changed=%v progress=%s", changed, progressString(stored.Progress))
	}
}

func TestPatchTaskStaleProgressLosesLWW(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertTask(ctx, sampleTask("user-1", base), true); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	winner := Patch{Progress: db.Field[*int]{Set: true, Value: intPtr(80)}}
	if _, _, err := s.patchTask(ctx, "task-1", "user-1", winner, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("patchTask (winner): %v", err)
	}

	// Ni un valor ni un null más viejos pisan el progress más nuevo.
	for _, stale := range []*int{intPtr(10), nil} {
		loser := Patch{Progress: db.Field[*int]{Set: true, Value: stale}}
		stored, changed, err := s.patchTask(ctx, "task-1", "user-1", loser, base.Add(time.Minute))
		if err != nil {
			t.Fatalf("patchTask (loser %s): %v", progressString(stale), err)
		}
		if changed || stored.Progress == nil || *stored.Progress != 80 {
			t.Fatalf("expected progress 80 to survive a stale %s, got changed=%v progress=%s",
				progressString(stale), changed, progressString(stored.Progress))
		}
	}
}

// Dos dispositivos editan campos distintos en paralelo: progress y
// status se resuelven cada uno por su timestamp, ninguno pisa al otro.
func TestPatchTaskProgressAndOtherFieldMergeIndependently(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertTask(ctx, sampleTask("user-1", base), true); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	progressPatch := Patch{Progress: db.Field[*int]{Set: true, Value: intPtr(50)}}
	if _, _, err := s.patchTask(ctx, "task-1", "user-1", progressPatch, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("patchTask (progress): %v", err)
	}

	// Llega después pero con un updated_at más viejo: status no se
	// había escrito nunca, así que gana; progress no se toca.
	statusPatch := Patch{Status: db.Field[string]{Set: true, Value: "in-progress"}}
	stored, changed, err := s.patchTask(ctx, "task-1", "user-1", statusPatch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchTask (status): %v", err)
	}
	if !changed || stored.Status != "in-progress" {
		t.Fatalf("expected status patch to apply, got changed=%v status=%q", changed, stored.Status)
	}
	if stored.Progress == nil || *stored.Progress != 50 {
		t.Fatalf("expected progress 50 untouched by the status patch, got %s", progressString(stored.Progress))
	}

	// Y al revés: un PATCH de progress no afecta al status ya resuelto.
	progressPatch = Patch{Progress: db.Field[*int]{Set: true, Value: intPtr(90)}}
	stored, _, err = s.patchTask(ctx, "task-1", "user-1", progressPatch, base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("patchTask (progress 2): %v", err)
	}
	if stored.Status != "in-progress" || stored.Progress == nil || *stored.Progress != 90 {
		t.Fatalf("expected status in-progress and progress 90, got status=%q progress=%s", stored.Status, progressString(stored.Progress))
	}
}

func TestChangesSinceIncludesProgress(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	in := sampleTask("user-1", base)
	in.Progress = intPtr(33)
	if _, err := s.upsertTask(ctx, in, true); err != nil {
		t.Fatalf("upsertTask: %v", err)
	}

	changes, err := ChangesSince(ctx, s.db, "user-1", 0)
	if err != nil {
		t.Fatalf("ChangesSince: %v", err)
	}
	if len(changes) != 1 || changes[0].Progress == nil || *changes[0].Progress != 33 {
		t.Fatalf("expected the pulled task to carry progress 33, got %+v", changes)
	}
}

// --- Handlers: validación y forma JSON ---

// newTestRouter monta create y patch sin el middleware de auth: estos
// tests miran la validación y el JSON, no la autenticación, así que
// el user_id queda vacío.
func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	h := NewHandler(newTestStore(t), nil, realtime.NewHub())
	r := chi.NewRouter()
	r.Post("/tasks", h.create)
	r.Patch("/tasks/{id}", h.patch)
	return r
}

func doJSON(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func createBody(progress string) string {
	body := `{"id":"task-1","title":"t","status":"todo","created":"2026-10-29T10:00:00.000Z","updated_at":"2026-10-29T10:00:00Z"`
	if progress != "" {
		body += `,"progress":` + progress
	}
	return body + "}"
}

func TestCreateHandlerProgressInResponse(t *testing.T) {
	cases := []struct {
		name     string
		progress string // "" = clave ausente
		want     string // valor JSON esperado en la respuesta
	}{
		{"con progress", "40", "40"},
		{"cero", "0", "0"},
		{"cien", "100", "100"},
		{"null explícito", "null", "null"},
		{"ausente", "", "null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := newTestRouter(t)
			rec := doJSON(t, router, http.MethodPost, "/tasks", createBody(tc.progress))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			var resp map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decoding response: %v", err)
			}
			got, ok := resp["progress"]
			if !ok {
				t.Fatalf("expected the response to always carry a progress key, got %s", rec.Body.String())
			}
			if string(got) != tc.want {
				t.Fatalf("expected progress %s, got %s", tc.want, got)
			}
		})
	}
}

var invalidProgressCases = []struct {
	name  string
	value string
}{
	{"negativo", "-1"},
	{"mayor que 100", "101"},
	{"decimal", "40.5"},
	{"decimal con .0", "40.0"},
	{"exponente", "1e2"},
	{"string", `"40"`},
	{"booleano", "true"},
	{"objeto", "{}"},
}

const wantProgressError = "progress must be an integer between 0 and 100, or null"

func TestCreateHandlerRejectsInvalidProgress(t *testing.T) {
	for _, tc := range invalidProgressCases {
		t.Run(tc.name, func(t *testing.T) {
			router := newTestRouter(t)
			rec := doJSON(t, router, http.MethodPost, "/tasks", createBody(tc.value))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := strings.TrimSpace(rec.Body.String()); got != wantProgressError {
				t.Fatalf("expected message %q, got %q", wantProgressError, got)
			}
		})
	}
}

func TestPatchHandlerProgress(t *testing.T) {
	router := newTestRouter(t)
	if rec := doJSON(t, router, http.MethodPost, "/tasks", createBody("10")); rec.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	steps := []struct {
		name string
		body string
		want string
	}{
		{"lo pone", `{"progress":55,"updated_at":"2026-10-29T10:01:00Z"}`, "55"},
		{"ausente no lo toca", `{"title":"otro","updated_at":"2026-10-29T10:02:00Z"}`, "55"},
		{"null lo borra", `{"progress":null,"updated_at":"2026-10-29T10:03:00Z"}`, "null"},
		{"más viejo no pisa", `{"progress":99,"updated_at":"2026-10-29T10:00:30Z"}`, "null"},
	}
	for _, st := range steps {
		rec := doJSON(t, router, http.MethodPatch, "/tasks/task-1", st.body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", st.name, rec.Code, rec.Body.String())
		}
		var resp map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: decoding response: %v", st.name, err)
		}
		if got := string(resp["progress"]); got != st.want {
			t.Fatalf("%s: expected progress %s, got %q", st.name, st.want, got)
		}
	}
}

func TestPatchHandlerRejectsInvalidProgress(t *testing.T) {
	for _, tc := range invalidProgressCases {
		t.Run(tc.name, func(t *testing.T) {
			router := newTestRouter(t)
			body := `{"progress":` + tc.value + `,"updated_at":"2026-10-29T10:01:00Z"}`
			rec := doJSON(t, router, http.MethodPatch, "/tasks/task-1", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := strings.TrimSpace(rec.Body.String()); got != wantProgressError {
				t.Fatalf("expected message %q, got %q", wantProgressError, got)
			}
		})
	}
}
