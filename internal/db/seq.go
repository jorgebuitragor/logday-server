package db

import (
	"context"
	"database/sql"
	"fmt"
)

// NextSeq atomically allocates and returns the next sync sequence
// number for userID (starting at 1), creating its counter row if
// needed. Run it in the same transaction as the row write it stamps,
// so the assigned seq and the write commit together.
func NextSeq(ctx context.Context, tx *sql.Tx, userID string) (int64, error) {
	var seq int64
	err := tx.QueryRowContext(ctx,
		`INSERT INTO user_sync_counters (user_id, next_seq) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET next_seq = next_seq + 1
		 RETURNING next_seq`,
		userID,
	).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("allocating sync seq: %w", err)
	}
	return seq, nil
}
