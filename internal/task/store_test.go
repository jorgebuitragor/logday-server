package task

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/db"
)

func newTestStore(t *testing.T) *store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return NewStore(database)
}

func sampleTask(userID string, updatedAt time.Time) *Task {
	return &Task{
		ID:        "task-1",
		UserID:    userID,
		Title:     "Write the sync protocol",
		Status:    "todo",
		Tags:      []string{"backend", "sync"},
		Project:   "logday-server",
		Created:   "2026-08-03",
		Content:   "some markdown",
		UpdatedAt: updatedAt,
	}
}

func TestUpsertTaskCreatesAndRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := sampleTask("user-1", time.Now())
	stored, err := s.upsertTask(ctx, in)
	if err != nil {
		t.Fatalf("upsertTask: %v", err)
	}
	if stored.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", stored.Seq)
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != in.Title || len(tasks[0].Tags) != 2 {
		t.Fatalf("unexpected list result: %+v", tasks)
	}
}

func TestUpsertTaskRejectsStaleWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertTask(ctx, sampleTask("user-1", base)); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	stale := sampleTask("user-1", base.Add(-time.Minute))
	stale.Title = "a stale edit"
	_, err := s.upsertTask(ctx, stale)
	if !errors.Is(err, errConflict) {
		t.Fatalf("expected errConflict, got %v", err)
	}

	newer := sampleTask("user-1", base.Add(time.Minute))
	newer.Title = "a newer edit"
	if _, err := s.upsertTask(ctx, newer); err != nil {
		t.Fatalf("newer upsertTask: %v", err)
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "a newer edit" {
		t.Fatalf("expected the newer edit to win, got %+v", tasks)
	}
}

func TestUpsertTaskRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertTask(ctx, sampleTask("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	other := sampleTask("user-2", time.Now().Add(time.Minute))
	_, err := s.upsertTask(ctx, other)
	if !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestSoftDeleteRemovesFromListAndChecksOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertTask(ctx, sampleTask("user-1", time.Now())); err != nil {
		t.Fatalf("upsertTask: %v", err)
	}

	if _, err := s.softDelete(ctx, "task-1", "user-2"); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden deleting as another user, got %v", err)
	}

	if _, err := s.softDelete(ctx, "task-1", "user-1"); err != nil {
		t.Fatalf("softDelete: %v", err)
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks after delete, got %+v", tasks)
	}

	if _, err := s.softDelete(ctx, "does-not-exist", "user-1"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestUpsertTaskAfterSoftDeleteResurrectsRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertTask(ctx, sampleTask("user-1", base)); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}
	if _, err := s.softDelete(ctx, "task-1", "user-1"); err != nil {
		t.Fatalf("softDelete: %v", err)
	}

	recreated := sampleTask("user-1", base.Add(time.Minute))
	recreated.Title = "recreated after delete"
	if _, err := s.upsertTask(ctx, recreated); err != nil {
		t.Fatalf("upsertTask after delete: %v", err)
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "recreated after delete" || tasks[0].DeletedAt != nil {
		t.Fatalf("expected the recreated task to be live again, got %+v", tasks)
	}

	// A second delete must succeed — the earlier ON CONFLICT DO UPDATE
	// bug left deleted_at set after recreation, so softDelete's
	// "WHERE deleted_at IS NULL" check would still find no live row.
	if _, err := s.softDelete(ctx, "task-1", "user-1"); err != nil {
		t.Fatalf("softDelete after resurrection: %v", err)
	}
}

func TestPatchTaskAppliesIndependentFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertTask(ctx, sampleTask("user-1", base)); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	// Two "devices" edit different fields concurrently — both should
	// survive, unlike whole-row LWW.
	titlePatch := Patch{Title: db.Field[string]{Set: true, Value: "new title"}}
	stored, changed, err := s.patchTask(ctx, "task-1", "user-1", titlePatch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchTask (title): %v", err)
	}
	if !changed || stored.Title != "new title" {
		t.Fatalf("expected title patch to apply, got changed=%v stored=%+v", changed, stored)
	}
	if stored.Status != "todo" {
		t.Fatalf("expected status untouched by title-only patch, got %q", stored.Status)
	}

	statusPatch := Patch{Status: db.Field[string]{Set: true, Value: "done"}}
	stored, changed, err = s.patchTask(ctx, "task-1", "user-1", statusPatch, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("patchTask (status): %v", err)
	}
	if !changed || stored.Status != "done" {
		t.Fatalf("expected status patch to apply, got changed=%v stored=%+v", changed, stored)
	}
	if stored.Title != "new title" {
		t.Fatalf("expected earlier title edit to survive, got %q", stored.Title)
	}
}

func TestPatchTaskDiscardsStaleFieldSilently(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertTask(ctx, sampleTask("user-1", base)); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	winner := Patch{Title: db.Field[string]{Set: true, Value: "winner"}}
	if _, _, err := s.patchTask(ctx, "task-1", "user-1", winner, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("patchTask (winner): %v", err)
	}

	// A write timestamped before the winner must not overwrite it, and
	// must not error — it's silently discarded (see specs/lww-por-campo).
	loser := Patch{Title: db.Field[string]{Set: true, Value: "loser"}}
	stored, changed, err := s.patchTask(ctx, "task-1", "user-1", loser, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchTask (loser): %v", err)
	}
	if changed {
		t.Fatalf("expected no change from a stale field write, got changed=true")
	}
	if stored.Title != "winner" {
		t.Fatalf("expected winner's title to survive, got %q", stored.Title)
	}
}

func TestPatchTaskAllFieldsStaleReturns200NoSeqBump(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertTask(ctx, sampleTask("user-1", base)); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}
	// Give "title" a real field_updated_at first — a completely fresh
	// row accepts any timestamp unconditionally (see
	// TestPatchTaskOnFreshRowAcceptsAnyTimestamp), so a "stale" write
	// against an untouched field wouldn't actually be stale.
	winner := Patch{Title: db.Field[string]{Set: true, Value: "already current"}}
	afterWinner, _, err := s.patchTask(ctx, "task-1", "user-1", winner, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("seeding winner patch: %v", err)
	}
	seqBefore := afterWinner.Seq

	stale := Patch{Title: db.Field[string]{Set: true, Value: "too late"}}
	stored, changed, err := s.patchTask(ctx, "task-1", "user-1", stale, base.Add(-time.Minute))
	if err != nil {
		t.Fatalf("patchTask: %v", err)
	}
	if changed {
		t.Fatalf("expected changed=false for an entirely stale patch")
	}
	if stored.Seq != seqBefore {
		t.Fatalf("expected seq unchanged (%d), got %d", seqBefore, stored.Seq)
	}
	if stored.Title != "already current" {
		t.Fatalf("expected winner's title to survive, got %q", stored.Title)
	}
}

func TestPatchTaskOnFreshRowAcceptsAnyTimestamp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertTask(ctx, sampleTask("user-1", base)); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	// field_updated_at is empty right after creation (POST doesn't
	// populate it) — the first PATCH to any field wins unconditionally,
	// even with an "old" timestamp relative to the row's updated_at.
	patch := Patch{Project: db.Field[string]{Set: true, Value: "new-project"}}
	stored, changed, err := s.patchTask(ctx, "task-1", "user-1", patch, base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("patchTask: %v", err)
	}
	if !changed || stored.Project != "new-project" {
		t.Fatalf("expected the first patch to a never-touched field to apply, got changed=%v stored=%+v", changed, stored)
	}
}

func TestPatchTaskRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertTask(ctx, sampleTask("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	patch := Patch{Title: db.Field[string]{Set: true, Value: "hijacked"}}
	_, _, err := s.patchTask(ctx, "task-1", "user-2", patch, time.Now())
	if !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestPatchTaskNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	patch := Patch{Title: db.Field[string]{Set: true, Value: "x"}}
	_, _, err := s.patchTask(ctx, "does-not-exist", "user-1", patch, time.Now())
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestPatchTaskNullableFieldExplicitNullClears(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	in := sampleTask("user-1", base)
	code := "TASK-1"
	in.TaskCode = &code
	if _, err := s.upsertTask(ctx, in); err != nil {
		t.Fatalf("initial upsertTask: %v", err)
	}

	patch := Patch{TaskCode: db.Field[*string]{Set: true, Value: nil}}
	stored, changed, err := s.patchTask(ctx, "task-1", "user-1", patch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchTask: %v", err)
	}
	if !changed || stored.TaskCode != nil {
		t.Fatalf("expected task_code cleared to nil, got changed=%v taskCode=%v", changed, stored.TaskCode)
	}
}

func TestListTasksIsScopedToUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertTask(ctx, sampleTask("user-1", time.Now())); err != nil {
		t.Fatalf("upsertTask: %v", err)
	}

	other := sampleTask("user-2", time.Now())
	other.ID = "task-2"
	if _, err := s.upsertTask(ctx, other); err != nil {
		t.Fatalf("upsertTask for user-2: %v", err)
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].UserID != "user-1" {
		t.Fatalf("expected only user-1's task, got %+v", tasks)
	}
}
