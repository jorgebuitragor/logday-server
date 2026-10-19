package note

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Deln0r/ygo"
)

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

func TestApplyContentUpdateFirstWrite(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertNote(ctx, sampleNote("user-1", time.Now())); err != nil {
		t.Fatalf("upsertNote: %v", err)
	}

	update := encodeInsert(t, nil, "hello world", 0)

	stored, err := s.applyContentUpdate(ctx, "note-1", "user-1", update, time.Now())
	if err != nil {
		t.Fatalf("applyContentUpdate: %v", err)
	}
	if stored.Content != "hello world" {
		t.Fatalf("expected content %q, got %q", "hello world", stored.Content)
	}
	if stored.ContentState == "" {
		t.Fatal("expected a non-empty content_state")
	}
	if stored.Seq != 2 { // 1 from upsertNote, 2 from applyContentUpdate
		t.Fatalf("expected seq 2, got %d", stored.Seq)
	}
}

// TestApplyContentUpdateMergesConcurrentEdits is the real-world case
// this whole feature exists for: two devices edit the same note's
// content offline, then both push to the server — neither edit is
// lost, exercised against the actual store (not just internal/crdt in
// isolation).
func TestApplyContentUpdateMergesConcurrentEdits(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertNote(ctx, sampleNote("user-1", time.Now())); err != nil {
		t.Fatalf("upsertNote: %v", err)
	}

	baseUpdate := encodeInsert(t, nil, "0123456789", 0)
	afterBase, err := s.applyContentUpdate(ctx, "note-1", "user-1", baseUpdate, time.Now())
	if err != nil {
		t.Fatalf("seeding base content: %v", err)
	}
	baseState, err := base64.StdEncoding.DecodeString(afterBase.ContentState)
	if err != nil {
		t.Fatalf("decoding base content_state: %v", err)
	}

	// Device A and device B both start from baseState, edit offline
	// (without seeing each other's edit), then push in sequence.
	updateA := encodeInsert(t, baseState, "AAA-", 0)
	updateB := encodeInsert(t, baseState, "-BBB", 10)

	if _, err := s.applyContentUpdate(ctx, "note-1", "user-1", updateA, time.Now()); err != nil {
		t.Fatalf("applying device A's update: %v", err)
	}
	afterB, err := s.applyContentUpdate(ctx, "note-1", "user-1", updateB, time.Now())
	if err != nil {
		t.Fatalf("applying device B's update: %v", err)
	}

	want := "AAA-0123456789-BBB"
	if afterB.Content != want {
		t.Fatalf("expected merged content %q, got %q", want, afterB.Content)
	}
}

func TestApplyContentUpdateRejectsCrossUserOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertNote(ctx, sampleNote("user-1", time.Now())); err != nil {
		t.Fatalf("upsertNote: %v", err)
	}

	update := encodeInsert(t, nil, "hello", 0)
	_, err := s.applyContentUpdate(ctx, "note-1", "user-2", update, time.Now())
	if !errors.Is(err, errForbidden) {
		t.Fatalf("expected errForbidden, got %v", err)
	}
}

func TestApplyContentUpdateNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	update := encodeInsert(t, nil, "hello", 0)
	_, err := s.applyContentUpdate(ctx, "does-not-exist", "user-1", update, time.Now())
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

// Regresión: crear una nota mientras se mezcla el contenido de otra (lo
// que hace Logday Web al pulsar "Nueva" con texto sin guardar) devolvía
// 500 cuando las dos transacciones chocaban: "database is locked". Ver
// TestConcurrentReadThenWriteTransactions en internal/db.
func TestConcurrentCreateAndContentUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.upsertNote(ctx, sampleNote("user-1", time.Now())); err != nil {
		t.Fatalf("upsertNote: %v", err)
	}

	const rounds = 30
	errs := make(chan error, 2*rounds)
	done := make(chan struct{})
	go func() {
		for i := 0; i < rounds; i++ {
			_, err := s.applyContentUpdate(ctx, "note-1", "user-1", encodeInsert(t, nil, "x", 0), time.Now())
			errs <- err
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < rounds; i++ {
			n := sampleNote("user-1", time.Now())
			n.ID = fmt.Sprintf("note-new-%d", i)
			_, err := s.upsertNote(ctx, n)
			errs <- err
		}
		done <- struct{}{}
	}()
	<-done
	<-done
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent write failed: %v", err)
		}
	}
	notes, err := s.listNotes(ctx, "user-1")
	if err != nil {
		t.Fatalf("listNotes: %v", err)
	}
	if len(notes) != rounds+1 {
		t.Fatalf("got %d notes, want %d", len(notes), rounds+1)
	}
}
