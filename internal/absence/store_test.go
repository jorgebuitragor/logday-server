package absence

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

func sampleDay(userID string, updatedAt time.Time) *Day {
	return &Day{
		ID:        "day-1",
		UserID:    userID,
		Date:      "2026-08-05",
		Type:      "vacaciones",
		UpdatedAt: updatedAt,
	}
}

func TestUpsertDayCreatesAndRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	stored, err := s.upsertDay(ctx, sampleDay("user-1", time.Now()))
	if err != nil {
		t.Fatalf("upsertDay: %v", err)
	}
	if stored.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", stored.Seq)
	}

	days, err := s.listDays(ctx, "user-1")
	if err != nil {
		t.Fatalf("listDays: %v", err)
	}
	if len(days) != 1 || days[0].Type != "vacaciones" {
		t.Fatalf("unexpected list result: %+v", days)
	}
}

func TestUpsertDayRejectsStaleWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertDay(ctx, sampleDay("user-1", base)); err != nil {
		t.Fatalf("initial upsertDay: %v", err)
	}

	stale := sampleDay("user-1", base.Add(-time.Minute))
	if _, err := s.upsertDay(ctx, stale); !errors.Is(err, errConflict) {
		t.Fatalf("expected errConflict, got %v", err)
	}
}

func TestUpsertDayRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertDay(ctx, sampleDay("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertDay: %v", err)
	}

	other := sampleDay("user-2", time.Now().Add(time.Minute))
	if _, err := s.upsertDay(ctx, other); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestSoftDeleteDayRemovesFromListAndChecksOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertDay(ctx, sampleDay("user-1", time.Now())); err != nil {
		t.Fatalf("upsertDay: %v", err)
	}

	if err := s.softDelete(ctx, "day-1", "user-2"); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
	if err := s.softDelete(ctx, "day-1", "user-1"); err != nil {
		t.Fatalf("softDelete: %v", err)
	}

	days, err := s.listDays(ctx, "user-1")
	if err != nil {
		t.Fatalf("listDays: %v", err)
	}
	if len(days) != 0 {
		t.Fatalf("expected no days after delete, got %+v", days)
	}

	if err := s.softDelete(ctx, "does-not-exist", "user-1"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}
