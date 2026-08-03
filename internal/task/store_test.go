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

func sampleTask(userID string, updatedAt time.Time) *task {
	return &task{
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

	if err := s.softDelete(ctx, "task-1", "user-2"); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden deleting as another user, got %v", err)
	}

	if err := s.softDelete(ctx, "task-1", "user-1"); err != nil {
		t.Fatalf("softDelete: %v", err)
	}

	tasks, err := s.listTasks(ctx, "user-1")
	if err != nil {
		t.Fatalf("listTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks after delete, got %+v", tasks)
	}

	if err := s.softDelete(ctx, "does-not-exist", "user-1"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
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
