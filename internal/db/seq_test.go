package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNextSeqAllocatesMonotonically(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i, want := range []int64{1, 2, 3} {
		got, err := NextSeq(ctx, tx, "user-1")
		if err != nil {
			t.Fatalf("NextSeq call %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("NextSeq call %d: got %d, want %d", i, got, want)
		}
	}

	// A different user gets its own independent counter.
	got, err := NextSeq(ctx, tx, "user-2")
	if err != nil {
		t.Fatalf("NextSeq for user-2: %v", err)
	}
	if got != 1 {
		t.Fatalf("NextSeq for user-2: got %d, want 1", got)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}
