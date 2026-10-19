package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
)

// Mismo patrón que cada store de dominio (upsertNote,
// applyContentUpdate, patchTask...): una transacción que primero lee y
// después escribe. Con transacciones DEFERRED en WAL, si otra escritura
// confirma entre la lectura y la escritura, la segunda falla al instante
// con SQLITE_BUSY ("database is locked") y el handler responde 500 —
// visto en el e2e de logday-web con un POST /notes en paralelo con un
// POST /notes/{id}/content. Ver specs/esquema-datos/design.md.
func TestConcurrentReadThenWriteTransactions(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	const workers, perWorker = 8, 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				errs <- readThenWrite(ctx, database)
			}
		}()
	}
	wg.Wait()
	close(errs)

	failed := 0
	var first error
	for err := range errs {
		if err != nil {
			if first == nil {
				first = err
			}
			failed++
		}
	}
	if failed > 0 {
		t.Fatalf("%d of %d concurrent transactions failed; first: %v", failed, workers*perWorker, first)
	}

	var next int64
	if err := database.QueryRowContext(ctx,
		`SELECT next_seq FROM user_sync_counters WHERE user_id = ?`, "user-1",
	).Scan(&next); err != nil {
		t.Fatalf("reading counter: %v", err)
	}
	if next != workers*perWorker {
		t.Fatalf("next_seq = %d, want %d (a lost update)", next, workers*perWorker)
	}
}

func readThenWrite(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_sync_counters`).Scan(&n); err != nil {
		return err
	}
	if _, err := NextSeq(ctx, tx, "user-1"); err != nil {
		return err
	}
	return tx.Commit()
}
