package overtime

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
		ID:         "entry-1",
		UserID:     userID,
		Fecha:      "2026-08-05",
		HoraInicio: "18:00",
		HoraFinal:  "20:00",
		TotalHoras: 2,
		UpdatedAt:  updatedAt,
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
	if len(entries) != 1 || entries[0].TotalHoras != 2 {
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

func TestUpsertEntryRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertEntry(ctx, sampleEntry("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertEntry: %v", err)
	}

	other := sampleEntry("user-2", time.Now().Add(time.Minute))
	if _, err := s.upsertEntry(ctx, other); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestSoftDeleteEntryChecksOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertEntry(ctx, sampleEntry("user-1", time.Now())); err != nil {
		t.Fatalf("upsertEntry: %v", err)
	}

	if _, err := s.softDeleteEntry(ctx, "entry-1", "user-2"); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
	if _, err := s.softDeleteEntry(ctx, "entry-1", "user-1"); err != nil {
		t.Fatalf("softDeleteEntry: %v", err)
	}

	entries, err := s.listEntries(ctx, "user-1")
	if err != nil {
		t.Fatalf("listEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries after delete, got %+v", entries)
	}
}

func sampleMonthMeta(userID string, updatedAt time.Time) *MonthMeta {
	return &MonthMeta{
		UserID:      userID,
		YearMonth:   "2026-08",
		Colaborador: "Jane Doe",
		Cedula:      "123456",
		UpdatedAt:   updatedAt,
	}
}

func TestUpsertMonthMetaCreatesAndRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	stored, err := s.upsertMonthMeta(ctx, sampleMonthMeta("user-1", time.Now()))
	if err != nil {
		t.Fatalf("upsertMonthMeta: %v", err)
	}
	if stored.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", stored.Seq)
	}

	records, err := s.listMonthMeta(ctx, "user-1")
	if err != nil {
		t.Fatalf("listMonthMeta: %v", err)
	}
	if len(records) != 1 || records[0].Colaborador != "Jane Doe" {
		t.Fatalf("unexpected list result: %+v", records)
	}
}

func TestUpsertMonthMetaRejectsStaleWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertMonthMeta(ctx, sampleMonthMeta("user-1", base)); err != nil {
		t.Fatalf("initial upsertMonthMeta: %v", err)
	}

	stale := sampleMonthMeta("user-1", base.Add(-time.Minute))
	if _, err := s.upsertMonthMeta(ctx, stale); !errors.Is(err, errConflict) {
		t.Fatalf("expected errConflict, got %v", err)
	}
}

func TestMonthMetaIsScopedToUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertMonthMeta(ctx, sampleMonthMeta("user-1", time.Now())); err != nil {
		t.Fatalf("upsertMonthMeta: %v", err)
	}
	if _, err := s.upsertMonthMeta(ctx, sampleMonthMeta("user-2", time.Now())); err != nil {
		t.Fatalf("upsertMonthMeta for user-2: %v", err)
	}

	records, err := s.listMonthMeta(ctx, "user-1")
	if err != nil {
		t.Fatalf("listMonthMeta: %v", err)
	}
	if len(records) != 1 || records[0].UserID != "user-1" {
		t.Fatalf("expected only user-1's record, got %+v", records)
	}
}
