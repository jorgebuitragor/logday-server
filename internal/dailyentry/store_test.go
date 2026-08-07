package dailyentry

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Deln0r/ygo"

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

func encodeInsert(t *testing.T, base []byte, text string, at uint64) []byte {
	t.Helper()
	doc := ygo.NewDoc()
	docText := ygo.NewText(doc, "content")
	if len(base) > 0 {
		if err := ygo.ApplyUpdate(doc, base); err != nil {
			t.Fatalf("loading base state: %v", err)
		}
	}
	txn := doc.WriteTxn()
	if err := docText.Insert(txn, at, text); err != nil {
		t.Fatalf("inserting text: %v", err)
	}
	txn.Commit()
	return ygo.EncodeStateAsUpdate(doc)
}

func TestApplyContentUpdateCreatesAndRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	update := encodeInsert(t, nil, "worked on daily entries", 0)

	stored, err := s.applyContentUpdate(ctx, "user-1", "2026-08-05", update, time.Now())
	if err != nil {
		t.Fatalf("applyContentUpdate: %v", err)
	}
	if stored.Content != "worked on daily entries" {
		t.Fatalf("expected content %q, got %q", "worked on daily entries", stored.Content)
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

// TestApplyContentUpdateMergesConcurrentEdits is the real-world case
// this feature exists for: two devices edit the same day's entry
// offline, then both push to the server — neither edit is lost.
func TestApplyContentUpdateMergesConcurrentEdits(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	baseUpdate := encodeInsert(t, nil, "0123456789", 0)
	afterBase, err := s.applyContentUpdate(ctx, "user-1", "2026-08-05", baseUpdate, time.Now())
	if err != nil {
		t.Fatalf("seeding base content: %v", err)
	}
	baseState, err := base64.StdEncoding.DecodeString(afterBase.ContentState)
	if err != nil {
		t.Fatalf("decoding base content_state: %v", err)
	}

	updateA := encodeInsert(t, baseState, "AAA-", 0)
	updateB := encodeInsert(t, baseState, "-BBB", 10)

	if _, err := s.applyContentUpdate(ctx, "user-1", "2026-08-05", updateA, time.Now()); err != nil {
		t.Fatalf("applying device A's update: %v", err)
	}
	afterB, err := s.applyContentUpdate(ctx, "user-1", "2026-08-05", updateB, time.Now())
	if err != nil {
		t.Fatalf("applying device B's update: %v", err)
	}

	want := "AAA-0123456789-BBB"
	if afterB.Content != want {
		t.Fatalf("expected merged content %q, got %q", want, afterB.Content)
	}
}

func TestEntriesAreScopedToUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	update1 := encodeInsert(t, nil, "user 1 entry", 0)
	if _, err := s.applyContentUpdate(ctx, "user-1", "2026-08-05", update1, time.Now()); err != nil {
		t.Fatalf("applyContentUpdate: %v", err)
	}
	update2 := encodeInsert(t, nil, "user 2 entry", 0)
	if _, err := s.applyContentUpdate(ctx, "user-2", "2026-08-05", update2, time.Now()); err != nil {
		t.Fatalf("applyContentUpdate for user-2: %v", err)
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

	update := encodeInsert(t, nil, "hello", 0)
	if _, err := s.applyContentUpdate(ctx, "user-1", "2026-08-05", update, time.Now()); err != nil {
		t.Fatalf("applyContentUpdate: %v", err)
	}

	if _, err := s.softDelete(ctx, "user-1", "2026-08-05"); err != nil {
		t.Fatalf("softDelete: %v", err)
	}

	entries, err := s.listEntries(ctx, "user-1")
	if err != nil {
		t.Fatalf("listEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries after delete, got %+v", entries)
	}

	if _, err := s.softDelete(ctx, "user-1", "does-not-exist"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}
