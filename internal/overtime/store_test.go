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

func TestPatchEntryAppliesIndependentFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertEntry(ctx, sampleEntry("user-1", base)); err != nil {
		t.Fatalf("initial upsertEntry: %v", err)
	}

	horasPatch := EntryPatch{TotalHoras: db.Field[float64]{Set: true, Value: 5}}
	stored, changed, err := s.patchEntry(ctx, "entry-1", "user-1", horasPatch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchEntry (total_horas): %v", err)
	}
	if !changed || stored.TotalHoras != 5 {
		t.Fatalf("expected total_horas patch to apply, got changed=%v stored=%+v", changed, stored)
	}

	actividadPatch := EntryPatch{Actividad: db.Field[string]{Set: true, Value: "deploy"}}
	stored, changed, err = s.patchEntry(ctx, "entry-1", "user-1", actividadPatch, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("patchEntry (actividad): %v", err)
	}
	if !changed || stored.Actividad != "deploy" {
		t.Fatalf("expected actividad patch to apply, got changed=%v stored=%+v", changed, stored)
	}
	if stored.TotalHoras != 5 {
		t.Fatalf("expected earlier total_horas edit to survive, got %v", stored.TotalHoras)
	}
}

func TestPatchEntryRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertEntry(ctx, sampleEntry("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertEntry: %v", err)
	}

	patch := EntryPatch{Actividad: db.Field[string]{Set: true, Value: "hijacked"}}
	_, _, err := s.patchEntry(ctx, "entry-1", "user-2", patch, time.Now())
	if !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestPatchEntryNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	patch := EntryPatch{Actividad: db.Field[string]{Set: true, Value: "x"}}
	_, _, err := s.patchEntry(ctx, "does-not-exist", "user-1", patch, time.Now())
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
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

func patchMonthMetaFull(s *store, ctx context.Context, m *MonthMeta) (*MonthMeta, bool, error) {
	patch := MonthMetaPatch{
		Colaborador: db.Field[string]{Set: true, Value: m.Colaborador},
		Cedula:      db.Field[string]{Set: true, Value: m.Cedula},
	}
	return s.patchMonthMeta(ctx, m.UserID, m.YearMonth, patch, m.UpdatedAt)
}

func TestPatchMonthMetaCreatesIfMissing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// overtime_month_meta has no POST (natural key, see
	// specs/esquema-datos) — PATCH must create the row on first write,
	// same as the PUT it replaces.
	stored, changed, err := patchMonthMetaFull(s, ctx, sampleMonthMeta("user-1", time.Now()))
	if err != nil {
		t.Fatalf("patchMonthMeta: %v", err)
	}
	if !changed || stored.Seq != 1 {
		t.Fatalf("expected a fresh row created, got changed=%v stored=%+v", changed, stored)
	}

	records, err := s.listMonthMeta(ctx, "user-1")
	if err != nil {
		t.Fatalf("listMonthMeta: %v", err)
	}
	if len(records) != 1 || records[0].Colaborador != "Jane Doe" {
		t.Fatalf("unexpected list result: %+v", records)
	}
}

func TestPatchMonthMetaAppliesIndependentFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, _, err := patchMonthMetaFull(s, ctx, sampleMonthMeta("user-1", base)); err != nil {
		t.Fatalf("initial patchMonthMeta: %v", err)
	}

	patch := MonthMetaPatch{Cedula: db.Field[string]{Set: true, Value: "999999"}}
	stored, changed, err := s.patchMonthMeta(ctx, "user-1", "2026-08", patch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchMonthMeta (cedula): %v", err)
	}
	if !changed || stored.Cedula != "999999" {
		t.Fatalf("expected cedula patch to apply, got changed=%v stored=%+v", changed, stored)
	}
	if stored.Colaborador != "Jane Doe" {
		t.Fatalf("expected colaborador untouched, got %q", stored.Colaborador)
	}
}

func TestPatchMonthMetaDiscardsStaleFieldSilently(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, _, err := patchMonthMetaFull(s, ctx, sampleMonthMeta("user-1", base)); err != nil {
		t.Fatalf("initial patchMonthMeta: %v", err)
	}

	winner := MonthMetaPatch{Colaborador: db.Field[string]{Set: true, Value: "winner"}}
	if _, _, err := s.patchMonthMeta(ctx, "user-1", "2026-08", winner, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("patchMonthMeta (winner): %v", err)
	}

	loser := MonthMetaPatch{Colaborador: db.Field[string]{Set: true, Value: "loser"}}
	stored, changed, err := s.patchMonthMeta(ctx, "user-1", "2026-08", loser, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchMonthMeta (loser): %v", err)
	}
	if changed {
		t.Fatalf("expected no change from a stale field write")
	}
	if stored.Colaborador != "winner" {
		t.Fatalf("expected winner's colaborador to survive, got %q", stored.Colaborador)
	}
}

func TestMonthMetaIsScopedToUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, _, err := patchMonthMetaFull(s, ctx, sampleMonthMeta("user-1", time.Now())); err != nil {
		t.Fatalf("patchMonthMeta: %v", err)
	}
	if _, _, err := patchMonthMetaFull(s, ctx, sampleMonthMeta("user-2", time.Now())); err != nil {
		t.Fatalf("patchMonthMeta for user-2: %v", err)
	}

	records, err := s.listMonthMeta(ctx, "user-1")
	if err != nil {
		t.Fatalf("listMonthMeta: %v", err)
	}
	if len(records) != 1 || records[0].UserID != "user-1" {
		t.Fatalf("expected only user-1's record, got %+v", records)
	}
}
