package note

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

// sampleNote covers the LWW-governed fields only — content is written
// exclusively through applyContentUpdate (see content_test.go), never
// through upsertNote.
func sampleNote(userID string, updatedAt time.Time) *Note {
	return &Note{
		ID:        "note-1",
		UserID:    userID,
		Title:     "Sync protocol notes",
		Folder:    "engineering",
		Tags:      []string{"sync"},
		Created:   "2026-08-03",
		Updated:   "2026-08-03",
		UpdatedAt: updatedAt,
	}
}

func TestUpsertNoteCreatesAndRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := sampleNote("user-1", time.Now())
	stored, err := s.upsertNote(ctx, in)
	if err != nil {
		t.Fatalf("upsertNote: %v", err)
	}
	if stored.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", stored.Seq)
	}

	notes, err := s.listNotes(ctx, "user-1")
	if err != nil {
		t.Fatalf("listNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].Title != in.Title || len(notes[0].Tags) != 1 {
		t.Fatalf("unexpected list result: %+v", notes)
	}
}

func TestUpsertNoteRejectsStaleWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertNote(ctx, sampleNote("user-1", base)); err != nil {
		t.Fatalf("initial upsertNote: %v", err)
	}

	stale := sampleNote("user-1", base.Add(-time.Minute))
	stale.Title = "a stale edit"
	_, err := s.upsertNote(ctx, stale)
	if !errors.Is(err, errConflict) {
		t.Fatalf("expected errConflict, got %v", err)
	}

	newer := sampleNote("user-1", base.Add(time.Minute))
	newer.Title = "a newer edit"
	if _, err := s.upsertNote(ctx, newer); err != nil {
		t.Fatalf("newer upsertNote: %v", err)
	}

	notes, err := s.listNotes(ctx, "user-1")
	if err != nil {
		t.Fatalf("listNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].Title != "a newer edit" {
		t.Fatalf("expected the newer edit to win, got %+v", notes)
	}
}

func TestUpsertNoteRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertNote(ctx, sampleNote("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertNote: %v", err)
	}

	other := sampleNote("user-2", time.Now().Add(time.Minute))
	_, err := s.upsertNote(ctx, other)
	if !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestSoftDeleteRemovesFromListAndChecksOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertNote(ctx, sampleNote("user-1", time.Now())); err != nil {
		t.Fatalf("upsertNote: %v", err)
	}

	if _, err := s.softDelete(ctx, "note-1", "user-2"); !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden deleting as another user, got %v", err)
	}

	if _, err := s.softDelete(ctx, "note-1", "user-1"); err != nil {
		t.Fatalf("softDelete: %v", err)
	}

	notes, err := s.listNotes(ctx, "user-1")
	if err != nil {
		t.Fatalf("listNotes: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("expected no notes after delete, got %+v", notes)
	}

	if _, err := s.softDelete(ctx, "does-not-exist", "user-1"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestPatchNoteAppliesIndependentFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertNote(ctx, sampleNote("user-1", base)); err != nil {
		t.Fatalf("initial upsertNote: %v", err)
	}

	titlePatch := Patch{Title: db.Field[string]{Set: true, Value: "new title"}}
	stored, changed, err := s.patchNote(ctx, "note-1", "user-1", titlePatch, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchNote (title): %v", err)
	}
	if !changed || stored.Title != "new title" {
		t.Fatalf("expected title patch to apply, got changed=%v stored=%+v", changed, stored)
	}

	folderPatch := Patch{Folder: db.Field[string]{Set: true, Value: "archive"}}
	stored, changed, err = s.patchNote(ctx, "note-1", "user-1", folderPatch, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("patchNote (folder): %v", err)
	}
	if !changed || stored.Folder != "archive" {
		t.Fatalf("expected folder patch to apply, got changed=%v stored=%+v", changed, stored)
	}
	if stored.Title != "new title" {
		t.Fatalf("expected earlier title edit to survive, got %q", stored.Title)
	}
}

func TestPatchNoteDiscardsStaleFieldSilently(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	base := time.Now()
	if _, err := s.upsertNote(ctx, sampleNote("user-1", base)); err != nil {
		t.Fatalf("initial upsertNote: %v", err)
	}

	winner := Patch{Title: db.Field[string]{Set: true, Value: "winner"}}
	if _, _, err := s.patchNote(ctx, "note-1", "user-1", winner, base.Add(2*time.Minute)); err != nil {
		t.Fatalf("patchNote (winner): %v", err)
	}

	loser := Patch{Title: db.Field[string]{Set: true, Value: "loser"}}
	stored, changed, err := s.patchNote(ctx, "note-1", "user-1", loser, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("patchNote (loser): %v", err)
	}
	if changed {
		t.Fatalf("expected no change from a stale field write")
	}
	if stored.Title != "winner" {
		t.Fatalf("expected winner's title to survive, got %q", stored.Title)
	}
}

func TestPatchNoteRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertNote(ctx, sampleNote("user-1", time.Now())); err != nil {
		t.Fatalf("initial upsertNote: %v", err)
	}

	patch := Patch{Title: db.Field[string]{Set: true, Value: "hijacked"}}
	_, _, err := s.patchNote(ctx, "note-1", "user-2", patch, time.Now())
	if !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestPatchNoteNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	patch := Patch{Title: db.Field[string]{Set: true, Value: "x"}}
	_, _, err := s.patchNote(ctx, "does-not-exist", "user-1", patch, time.Now())
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestListNotesIsScopedToUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertNote(ctx, sampleNote("user-1", time.Now())); err != nil {
		t.Fatalf("upsertNote: %v", err)
	}

	other := sampleNote("user-2", time.Now())
	other.ID = "note-2"
	if _, err := s.upsertNote(ctx, other); err != nil {
		t.Fatalf("upsertNote for user-2: %v", err)
	}

	notes, err := s.listNotes(ctx, "user-1")
	if err != nil {
		t.Fatalf("listNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].UserID != "user-1" {
		t.Fatalf("expected only user-1's note, got %+v", notes)
	}
}
