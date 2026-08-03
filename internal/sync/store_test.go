package sync

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/db"
)

func newTestStore(t *testing.T) (*store, *sql.DB) {
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
	return NewStore(database), database
}

// insertTaskFixture writes a task row directly via SQL, bypassing
// internal/task's store (unexported, not reachable from this
// package) — this test exercises sync's aggregation, not task's write
// logic, which already has its own tests.
func insertTaskFixture(t *testing.T, database *sql.DB, id, userID string, seq int64, deleted bool) {
	t.Helper()
	var deletedAt any
	if deleted {
		deletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := database.Exec(`
		INSERT INTO tasks (id, user_id, title, status, tags, project, created, content, seq, updated_at, deleted_at)
		VALUES (?, ?, ?, 'todo', '[]', '', '2026-08-03', '', ?, ?, ?)
	`, id, userID, "Task "+id, seq, time.Now().UTC().Format(time.RFC3339Nano), deletedAt)
	if err != nil {
		t.Fatalf("inserting task fixture: %v", err)
	}
}

func TestChangesSinceReturnsInSeqOrder(t *testing.T) {
	s, database := newTestStore(t)
	ctx := context.Background()

	insertTaskFixture(t, database, "task-2", "user-1", 2, false)
	insertTaskFixture(t, database, "task-1", "user-1", 1, false)
	insertTaskFixture(t, database, "task-3", "user-1", 3, false)

	changes, err := s.changesSince(ctx, "user-1", 0)
	if err != nil {
		t.Fatalf("changesSince: %v", err)
	}
	if len(changes) != 3 {
		t.Fatalf("expected 3 changes, got %d", len(changes))
	}
	for i, want := range []string{"task-1", "task-2", "task-3"} {
		if changes[i].ID != want || changes[i].Type != "task" {
			t.Fatalf("changes[%d] = %+v, want id %q", i, changes[i], want)
		}
	}
}

func TestChangesSinceFiltersByCursor(t *testing.T) {
	s, database := newTestStore(t)
	ctx := context.Background()

	insertTaskFixture(t, database, "task-1", "user-1", 1, false)
	insertTaskFixture(t, database, "task-2", "user-1", 2, false)

	changes, err := s.changesSince(ctx, "user-1", 1)
	if err != nil {
		t.Fatalf("changesSince: %v", err)
	}
	if len(changes) != 1 || changes[0].ID != "task-2" {
		t.Fatalf("expected only task-2 after cursor=1, got %+v", changes)
	}
}

func TestChangesSinceSurfacesTombstones(t *testing.T) {
	s, database := newTestStore(t)
	ctx := context.Background()

	insertTaskFixture(t, database, "task-1", "user-1", 1, true)

	changes, err := s.changesSince(ctx, "user-1", 0)
	if err != nil {
		t.Fatalf("changesSince: %v", err)
	}
	if len(changes) != 1 || !changes[0].Deleted {
		t.Fatalf("expected a deleted change, got %+v", changes)
	}
}

func TestChangesSinceIsScopedToUser(t *testing.T) {
	s, database := newTestStore(t)
	ctx := context.Background()

	insertTaskFixture(t, database, "task-1", "user-1", 1, false)
	insertTaskFixture(t, database, "task-2", "user-2", 1, false)

	changes, err := s.changesSince(ctx, "user-1", 0)
	if err != nil {
		t.Fatalf("changesSince: %v", err)
	}
	if len(changes) != 1 || changes[0].ID != "task-1" {
		t.Fatalf("expected only user-1's task, got %+v", changes)
	}
}
