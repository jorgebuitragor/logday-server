package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return database
}

// insertTombstoneFixture writes a soft-deleted task row directly via
// SQL. It also seeds user_sync_counters for userID, since in real
// usage that row always exists by the time a tombstone does (every
// soft-delete goes through db.NextSeq first) — this fixture bypasses
// that path, so it has to replicate the precondition explicitly.
func insertTombstoneFixture(t *testing.T, database *sql.DB, userID, id string, seq int64, deletedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := database.Exec(`
		INSERT INTO tasks (id, user_id, title, status, tags, project, created, content, seq, updated_at, deleted_at)
		VALUES (?, ?, ?, 'todo', '[]', '', '2026-08-06', '', ?, ?, ?)
	`, id, userID, "Task "+id, seq, now, deletedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("inserting tombstone fixture: %v", err)
	}

	_, err = database.Exec(`
		INSERT INTO user_sync_counters (user_id, next_seq) VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET next_seq = MAX(next_seq, excluded.next_seq)
	`, userID, seq+1)
	if err != nil {
		t.Fatalf("seeding user_sync_counters fixture: %v", err)
	}
}

func TestPurgeTombstonesRemovesOldOnesAndKeepsRecent(t *testing.T) {
	database := newTestDB(t)
	ctx := context.Background()

	insertTombstoneFixture(t, database, "user-1", "old", 1, time.Now().Add(-91*24*time.Hour))
	insertTombstoneFixture(t, database, "user-1", "recent", 2, time.Now().Add(-1*time.Hour))

	if err := PurgeTombstones(ctx, database); err != nil {
		t.Fatalf("PurgeTombstones: %v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id = 'old'`).Scan(&count); err != nil {
		t.Fatalf("querying old task: %v", err)
	}
	if count != 0 {
		t.Fatal("expected the old tombstone to be physically deleted")
	}

	if err := database.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id = 'recent'`).Scan(&count); err != nil {
		t.Fatalf("querying recent task: %v", err)
	}
	if count != 1 {
		t.Fatal("expected the recent tombstone to survive the purge")
	}
}

func TestPurgeTombstonesSetsWatermark(t *testing.T) {
	database := newTestDB(t)
	ctx := context.Background()

	insertTombstoneFixture(t, database, "user-1", "old-1", 5, time.Now().Add(-100*24*time.Hour))
	insertTombstoneFixture(t, database, "user-1", "old-2", 9, time.Now().Add(-95*24*time.Hour))

	watermark, err := PurgedBeforeSeq(ctx, database, "user-1")
	if err != nil {
		t.Fatalf("PurgedBeforeSeq before purge: %v", err)
	}
	if watermark != 0 {
		t.Fatalf("expected watermark 0 before purge, got %d", watermark)
	}

	if err := PurgeTombstones(ctx, database); err != nil {
		t.Fatalf("PurgeTombstones: %v", err)
	}

	watermark, err = PurgedBeforeSeq(ctx, database, "user-1")
	if err != nil {
		t.Fatalf("PurgedBeforeSeq after purge: %v", err)
	}
	if watermark != 9 {
		t.Fatalf("expected watermark 9 (the higher of the two purged seqs), got %d", watermark)
	}
}

func TestPurgeTombstonesWatermarkNeverDecreases(t *testing.T) {
	database := newTestDB(t)
	ctx := context.Background()

	insertTombstoneFixture(t, database, "user-1", "old-1", 10, time.Now().Add(-100*24*time.Hour))
	if err := PurgeTombstones(ctx, database); err != nil {
		t.Fatalf("first PurgeTombstones: %v", err)
	}

	// A second purge run with nothing new eligible must not lower the
	// watermark back down.
	if err := PurgeTombstones(ctx, database); err != nil {
		t.Fatalf("second PurgeTombstones: %v", err)
	}

	watermark, err := PurgedBeforeSeq(ctx, database, "user-1")
	if err != nil {
		t.Fatalf("PurgedBeforeSeq: %v", err)
	}
	if watermark != 10 {
		t.Fatalf("expected watermark to stay at 10, got %d", watermark)
	}
}

func TestPurgedBeforeSeqUnknownUserReturnsZero(t *testing.T) {
	database := newTestDB(t)
	ctx := context.Background()

	watermark, err := PurgedBeforeSeq(ctx, database, "no-such-user")
	if err != nil {
		t.Fatalf("PurgedBeforeSeq: %v", err)
	}
	if watermark != 0 {
		t.Fatalf("expected 0 for an unknown user, got %d", watermark)
	}
}
