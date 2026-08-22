package calendar

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

func sampleEvent(userID string, updatedAt time.Time) *Event {
	return &Event{
		ID:        "event-1",
		UserID:    userID,
		Title:     "Standup",
		Date:      "2026-08-05",
		Color:     "indigo",
		Repeat:    "daily",
		UpdatedAt: updatedAt,
	}
}

func TestUpsertEventCreatesAndRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	stored, err := s.upsertEvent(ctx, sampleEvent("user-1", time.Now()))
	if err != nil {
		t.Fatalf("upsertEvent: %v", err)
	}
	if stored.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", stored.Seq)
	}

	events, err := s.listEvents(ctx, "user-1")
	if err != nil {
		t.Fatalf("listEvents: %v", err)
	}
	if len(events) != 1 || events[0].Title != "Standup" {
		t.Fatalf("unexpected list result: %+v", events)
	}
}

func TestUpsertEventRejectsStaleWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertEvent(ctx, sampleEvent("user-1", base)); err != nil {
		t.Fatalf("initial upsertEvent: %v", err)
	}

	stale := sampleEvent("user-1", base.Add(-time.Minute))
	if _, err := s.upsertEvent(ctx, stale); !errors.Is(err, errConflict) {
		t.Fatalf("expected errConflict, got %v", err)
	}
}

func TestUpsertEventRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertEvent(ctx, sampleEvent("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertEvent: %v", err)
	}

	other := sampleEvent("user-2", time.Now().Add(time.Minute))
	if _, err := s.upsertEvent(ctx, other); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestPatchEventAppliesIndependentFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertEvent(ctx, sampleEvent("user-1", base)); err != nil {
		t.Fatalf("initial upsertEvent: %v", err)
	}

	titlePatch := Patch{Title: db.Field[string]{Set: true, Value: "new title"}}
	stored, changed, err := s.patchEvent(ctx, "event-1", "user-1", titlePatch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchEvent (title): %v", err)
	}
	if !changed || stored.Title != "new title" {
		t.Fatalf("expected title patch to apply, got changed=%v stored=%+v", changed, stored)
	}

	colorPatch := Patch{Color: db.Field[string]{Set: true, Value: "rose"}}
	stored, changed, err = s.patchEvent(ctx, "event-1", "user-1", colorPatch, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("patchEvent (color): %v", err)
	}
	if !changed || stored.Color != "rose" {
		t.Fatalf("expected color patch to apply, got changed=%v stored=%+v", changed, stored)
	}
	if stored.Title != "new title" {
		t.Fatalf("expected earlier title edit to survive, got %q", stored.Title)
	}
}

func TestPatchEventDiscardsStaleFieldSilently(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertEvent(ctx, sampleEvent("user-1", base)); err != nil {
		t.Fatalf("initial upsertEvent: %v", err)
	}

	winner := Patch{Title: db.Field[string]{Set: true, Value: "winner"}}
	if _, _, err := s.patchEvent(ctx, "event-1", "user-1", winner, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("patchEvent (winner): %v", err)
	}

	loser := Patch{Title: db.Field[string]{Set: true, Value: "loser"}}
	stored, changed, err := s.patchEvent(ctx, "event-1", "user-1", loser, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchEvent (loser): %v", err)
	}
	if changed {
		t.Fatalf("expected no change from a stale field write")
	}
	if stored.Title != "winner" {
		t.Fatalf("expected winner's title to survive, got %q", stored.Title)
	}
}

func TestPatchEventRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertEvent(ctx, sampleEvent("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertEvent: %v", err)
	}

	patch := Patch{Title: db.Field[string]{Set: true, Value: "hijacked"}}
	_, _, err := s.patchEvent(ctx, "event-1", "user-2", patch, time.Now())
	if !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestPatchEventNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	patch := Patch{Title: db.Field[string]{Set: true, Value: "x"}}
	_, _, err := s.patchEvent(ctx, "does-not-exist", "user-1", patch, time.Now())
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestSoftDeleteEventRemovesFromListAndChecksOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertEvent(ctx, sampleEvent("user-1", time.Now())); err != nil {
		t.Fatalf("upsertEvent: %v", err)
	}

	if _, err := s.softDelete(ctx, "event-1", "user-2"); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
	if _, err := s.softDelete(ctx, "event-1", "user-1"); err != nil {
		t.Fatalf("softDelete: %v", err)
	}

	events, err := s.listEvents(ctx, "user-1")
	if err != nil {
		t.Fatalf("listEvents: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events after delete, got %+v", events)
	}

	if _, err := s.softDelete(ctx, "does-not-exist", "user-1"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}
