package dailyentry

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

func sampleEntry(userID string, updatedAt time.Time) *Entry {
	return &Entry{
		UserID:    userID,
		Date:      "2026-08-05",
		Content:   "worked on daily entries",
		UpdatedAt: updatedAt,
	}
}

func TestUpsertEntryCreatesAndRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	stored, err := s.upsertEntry(ctx, sampleEntry("user-1", time.Now()))
	if err != nil {
		t.Fatalf("upsertEntry: %v", err)
	}
	if stored.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", stored.Seq)
	}

	entries, err := s.listEntries(ctx, "user-1")
	if err != nil {
		t.Fatalf("listEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "worked on daily entries" {
		t.Fatalf("unexpected list result: %+v", entries)
	}
}

func TestUpsertEntryRejectsStaleWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertEntry(ctx, sampleEntry("user-1", base)); err != nil {
		t.Fatalf("initial upsertEntry: %v", err)
	}

	stale := sampleEntry("user-1", base.Add(-time.Minute))
	if _, err := s.upsertEntry(ctx, stale); !errors.Is(err, errConflict) {
		t.Fatalf("expected errConflict, got %v", err)
	}
}

func TestEntriesAreScopedToUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertEntry(ctx, sampleEntry("user-1", time.Now())); err != nil {
		t.Fatalf("upsertEntry: %v", err)
	}
	if _, err := s.upsertEntry(ctx, sampleEntry("user-2", time.Now())); err != nil {
		t.Fatalf("upsertEntry for user-2: %v", err)
	}

	entries, err := s.listEntries(ctx, "user-1")
	if err != nil {
		t.Fatalf("listEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].UserID != "user-1" {
		t.Fatalf("expected only user-1's entry, got %+v", entries)
	}
}

func TestSoftDeleteEntry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertEntry(ctx, sampleEntry("user-1", time.Now())); err != nil {
		t.Fatalf("upsertEntry: %v", err)
	}

	if err := s.softDelete(ctx, "user-1", "2026-08-05"); err != nil {
		t.Fatalf("softDelete: %v", err)
	}

	entries, err := s.listEntries(ctx, "user-1")
	if err != nil {
		t.Fatalf("listEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries after delete, got %+v", entries)
	}

	if err := s.softDelete(ctx, "user-1", "does-not-exist"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}
