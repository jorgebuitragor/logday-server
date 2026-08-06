package sync

import (
	"context"
	"database/sql"
	"errors"
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

// TestChangesSinceUsesNaturalKeyAsID covers the two entities without a
// client-generated id (overtime_month_meta, daily_entries) — their
// change envelope should use the natural key (year_month, date) as
// the synthetic id, per specs/esquema-datos.
func TestChangesSinceUsesNaturalKeyAsID(t *testing.T) {
	s, database := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`
		INSERT INTO overtime_month_meta (user_id, year_month, colaborador, cedula, seq, updated_at)
		VALUES ('user-1', '2026-08', 'Jane Doe', '123456', 1, ?)
	`, now)
	if err != nil {
		t.Fatalf("inserting overtime_month_meta fixture: %v", err)
	}
	_, err = database.Exec(`
		INSERT INTO daily_entries (user_id, date, content, seq, updated_at)
		VALUES ('user-1', '2026-08-05', 'worked on sync', 2, ?)
	`, now)
	if err != nil {
		t.Fatalf("inserting daily_entries fixture: %v", err)
	}

	changes, err := s.changesSince(ctx, "user-1", 0)
	if err != nil {
		t.Fatalf("changesSince: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(changes), changes)
	}
	if changes[0].Type != "overtime_month_meta" || changes[0].ID != "2026-08" {
		t.Fatalf("changes[0] = %+v, want type overtime_month_meta id 2026-08", changes[0])
	}
	if changes[1].Type != "daily_entry" || changes[1].ID != "2026-08-05" {
		t.Fatalf("changes[1] = %+v, want type daily_entry id 2026-08-05", changes[1])
	}
}

// TestChangesSinceRejectsCursorOlderThanPurgeWatermark covers the
// 410-equivalent case: since predates a tombstone that got physically
// purged, so an incremental answer would silently omit a delete the
// client needs to see.
func TestChangesSinceRejectsCursorOlderThanPurgeWatermark(t *testing.T) {
	s, database := newTestStore(t)
	ctx := context.Background()

	insertTaskFixture(t, database, "old-tombstone", "user-1", 5, true)
	// PurgeTombstones only purges rows older than the retention
	// window, but insertTaskFixture always stamps deleted_at = now.
	// Backdate it directly so this fixture is actually eligible.
	if _, err := database.Exec(
		`UPDATE tasks SET deleted_at = ? WHERE id = 'old-tombstone'`,
		time.Now().Add(-100*24*time.Hour).UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("backdating tombstone fixture: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO user_sync_counters (user_id, next_seq) VALUES ('user-1', 6)
		 ON CONFLICT(user_id) DO UPDATE SET next_seq = MAX(next_seq, 6)`,
	); err != nil {
		t.Fatalf("seeding user_sync_counters fixture: %v", err)
	}

	if err := db.PurgeTombstones(ctx, database); err != nil {
		t.Fatalf("PurgeTombstones: %v", err)
	}

	if _, err := s.changesSince(ctx, "user-1", 2); !errors.Is(err, errCursorInvalid) {
		t.Fatalf("expected errCursorInvalid for since=2 (below watermark), got %v", err)
	}

	if _, err := s.changesSince(ctx, "user-1", 5); err != nil {
		t.Fatalf("expected since=5 (at watermark) to succeed, got %v", err)
	}
}
