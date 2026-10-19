package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// Open connects to the SQLite database at path, creating its parent
// directory and the database file if they don't already exist.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("creating db directory: %w", err)
		}
	}

	// Every BeginTx in the domain stores reads and then writes (check the
	// owner/updated_at, then upsert; read content_crdt, merge, store). As
	// a DEFERRED transaction that starts as a reader, and if another
	// write commits in between, SQLite refuses to upgrade it to a writer
	// and fails right away with SQLITE_BUSY ("database is locked") — the
	// busy handler is never consulted for that upgrade, so busy_timeout
	// alone doesn't help. _txlock=immediate makes BeginTx take the write
	// lock up front (BEGIN IMMEDIATE), and _busy_timeout makes a second
	// writer wait for it instead of failing. Plain reads outside a
	// transaction (db.Query*) are unaffected: WAL keeps them concurrent.
	// See TestConcurrentReadThenWriteTransactions.
	database, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return database, nil
}
