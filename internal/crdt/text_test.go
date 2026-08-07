package crdt

import (
	"testing"

	"github.com/Deln0r/ygo"
)

func TestTextEmptyState(t *testing.T) {
	text, err := Text(nil)
	if err != nil {
		t.Fatalf("Text(nil): %v", err)
	}
	if text != "" {
		t.Fatalf("expected empty text, got %q", text)
	}
}

func TestApplyTextUpdateFirstWrite(t *testing.T) {
	client := ygo.NewDoc()
	clientText := ygo.NewText(client, textKey)
	txn := client.WriteTxn()
	if err := clientText.Insert(txn, 0, "hello world"); err != nil {
		t.Fatalf("client insert: %v", err)
	}
	txn.Commit()

	update := ygo.EncodeStateAsUpdate(client)

	state, text, err := ApplyTextUpdate(nil, update)
	if err != nil {
		t.Fatalf("ApplyTextUpdate: %v", err)
	}
	if text != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", text)
	}
	if len(state) == 0 {
		t.Fatal("expected non-empty stored state")
	}

	// Text() on the stored state must decode the same text.
	decoded, err := Text(state)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if decoded != "hello world" {
		t.Fatalf("Text(state) = %q, want %q", decoded, "hello world")
	}
}

func TestApplyTextUpdateMergesConcurrentEdits(t *testing.T) {
	// Simulate the server already holding a base state.
	base := ygo.NewDoc()
	baseText := ygo.NewText(base, textKey)
	txn := base.WriteTxn()
	if err := baseText.Insert(txn, 0, "0123456789"); err != nil {
		t.Fatalf("base insert: %v", err)
	}
	txn.Commit()
	baseUpdate := ygo.EncodeStateAsUpdate(base)

	state, _, err := ApplyTextUpdate(nil, baseUpdate)
	if err != nil {
		t.Fatalf("seeding base state: %v", err)
	}

	// Device A: starts from base, inserts a prefix.
	a := ygo.NewDoc()
	aText := ygo.NewText(a, textKey)
	if err := ygo.ApplyUpdate(a, baseUpdate); err != nil {
		t.Fatalf("device A loading base: %v", err)
	}
	txnA := a.WriteTxn()
	if err := aText.Insert(txnA, 0, "AAA-"); err != nil {
		t.Fatalf("device A insert: %v", err)
	}
	txnA.Commit()
	updateA := ygo.EncodeStateAsUpdate(a)

	// Device B: starts from base too, inserts a suffix, without
	// having seen device A's edit yet (both offline).
	b := ygo.NewDoc()
	bText := ygo.NewText(b, textKey)
	if err := ygo.ApplyUpdate(b, baseUpdate); err != nil {
		t.Fatalf("device B loading base: %v", err)
	}
	txnB := b.WriteTxn()
	if err := bText.Insert(txnB, bText.Length(), "-BBB"); err != nil {
		t.Fatalf("device B insert: %v", err)
	}
	txnB.Commit()
	updateB := ygo.EncodeStateAsUpdate(b)

	// Server receives both updates, one after the other.
	state, textAfterA, err := ApplyTextUpdate(state, updateA)
	if err != nil {
		t.Fatalf("applying device A's update: %v", err)
	}
	state, textAfterB, err := ApplyTextUpdate(state, updateB)
	if err != nil {
		t.Fatalf("applying device B's update: %v", err)
	}

	want := "AAA-0123456789-BBB"
	if textAfterB != want {
		t.Fatalf("after merging both edits, got %q, want %q (in-between state was %q)",
			textAfterB, want, textAfterA)
	}

	// The final stored state must decode consistently too.
	decoded, err := Text(state)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if decoded != want {
		t.Fatalf("Text(final state) = %q, want %q", decoded, want)
	}
}

func TestApplyTextUpdateIsIdempotent(t *testing.T) {
	client := ygo.NewDoc()
	clientText := ygo.NewText(client, textKey)
	txn := client.WriteTxn()
	if err := clientText.Insert(txn, 0, "hello"); err != nil {
		t.Fatalf("client insert: %v", err)
	}
	txn.Commit()
	update := ygo.EncodeStateAsUpdate(client)

	state, text1, err := ApplyTextUpdate(nil, update)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Re-applying the exact same update (e.g. a client retry after a
	// dropped response) must be a safe no-op, not duplicate the text.
	_, text2, err := ApplyTextUpdate(state, update)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if text1 != text2 || text2 != "hello" {
		t.Fatalf("re-applying the same update changed the text: %q -> %q", text1, text2)
	}
}
