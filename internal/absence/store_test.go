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

func TestPatchDayAppliesIndependentFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertDay(ctx, sampleDay("user-1", base)); err != nil {
		t.Fatalf("initial upsertDay: %v", err)
	}

	typePatch := Patch{Type: db.Field[string]{Set: true, Value: "incapacidad"}}
	stored, changed, err := s.patchDay(ctx, "day-1", "user-1", typePatch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchDay (type): %v", err)
	}
	if !changed || stored.Type != "incapacidad" {
		t.Fatalf("expected type patch to apply, got changed=%v stored=%+v", changed, stored)
	}

	note := "certificado adjunto"
	notePatch := Patch{Note: db.Field[*string]{Set: true, Value: &note}}
	stored, changed, err = s.patchDay(ctx, "day-1", "user-1", notePatch, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("patchDay (note): %v", err)
	}
	if !changed || stored.Note == nil || *stored.Note != note {
		t.Fatalf("expected note patch to apply, got changed=%v stored=%+v", changed, stored)
	}
	if stored.Type != "incapacidad" {
		t.Fatalf("expected earlier type edit to survive, got %q", stored.Type)
	}
}

func TestPatchDayNullableFieldExplicitNullClears(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	in := sampleDay("user-1", base)
	note := "algo"
	in.Note = &note
	if _, err := s.upsertDay(ctx, in); err != nil {
		t.Fatalf("initial upsertDay: %v", err)
	}

	patch := Patch{Note: db.Field[*string]{Set: true, Value: nil}}
	stored, changed, err := s.patchDay(ctx, "day-1", "user-1", patch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchDay: %v", err)
	}
	if !changed || stored.Note != nil {
		t.Fatalf("expected note cleared to nil, got changed=%v note=%v", changed, stored.Note)
	}
}

func TestPatchDayDiscardsStaleFieldSilently(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertDay(ctx, sampleDay("user-1", base)); err != nil {
		t.Fatalf("initial upsertDay: %v", err)
	}

	winner := Patch{Type: db.Field[string]{Set: true, Value: "otro"}}
	if _, _, err := s.patchDay(ctx, "day-1", "user-1", winner, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("patchDay (winner): %v", err)
	}

	loser := Patch{Type: db.Field[string]{Set: true, Value: "incapacidad"}}
	stored, changed, err := s.patchDay(ctx, "day-1", "user-1", loser, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchDay (loser): %v", err)
	}
	if changed {
		t.Fatalf("expected no change from a stale field write")
	}
	if stored.Type != "otro" {
		t.Fatalf("expected winner's type to survive, got %q", stored.Type)
	}
}

func TestPatchDayRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertDay(ctx, sampleDay("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertDay: %v", err)
	}

	patch := Patch{Type: db.Field[string]{Set: true, Value: "otro"}}
	_, _, err := s.patchDay(ctx, "day-1", "user-2", patch, time.Now())
	if !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestPatchDayNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	patch := Patch{Type: db.Field[string]{Set: true, Value: "otro"}}
	_, _, err := s.patchDay(ctx, "does-not-exist", "user-1", patch, time.Now())
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestSoftDeleteDayRemovesFromListAndChecksOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertDay(ctx, sampleDay("user-1", time.Now())); err != nil {
		t.Fatalf("upsertDay: %v", err)
	}

	if _, err := s.softDelete(ctx, "day-1", "user-2"); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
	if _, err := s.softDelete(ctx, "day-1", "user-1"); err != nil {
		t.Fatalf("softDelete: %v", err)
	}

	days, err := s.listDays(ctx, "user-1")
	if err != nil {
		t.Fatalf("listDays: %v", err)
	}
	if len(days) != 0 {
		t.Fatalf("expected no days after delete, got %+v", days)
	}

	if _, err := s.softDelete(ctx, "does-not-exist", "user-1"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}
