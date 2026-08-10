package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/settings"
)

// domainTables lists every table with the standard
// (user_id, seq, deleted_at) shape that participates in soft-delete +
// purge — see specs/esquema-datos.
var domainTables = []string{
	"tasks", "notes", "overtime_entries", "overtime_month_meta",
	"calendar_events", "absence_days", "daily_entries",
}

// PurgeTombstones physically deletes soft-deleted rows older than the
// retention window (instance_settings.tombstone_retention_days,
// configurable from the admin panel — 90 days by default) from every
// domain table, recording per-user how far the purge reached
// (user_sync_counters.purged_before_seq) so GET /sync/changes can
// detect a cursor that's now too old to answer completely and reject
// it (410) instead of silently omitting the tombstones it missed.
func PurgeTombstones(ctx context.Context, database *sql.DB) error {
	cfg, err := settings.Get(ctx, database)
	if err != nil {
		return fmt.Errorf("reading tombstone retention setting: %w", err)
	}
	cutoff := time.Now().UTC().Add(-cfg.TombstoneRetention()).Format(time.RFC3339Nano)

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	watermarks, err := tombstoneWatermarks(ctx, tx, cutoff)
	if err != nil {
		return err
	}

	for userID, seq := range watermarks {
		if _, err := tx.ExecContext(ctx,
			`UPDATE user_sync_counters SET purged_before_seq = MAX(purged_before_seq, ?) WHERE user_id = ?`,
			seq, userID,
		); err != nil {
			return fmt.Errorf("updating purge watermark for %s: %w", userID, err)
		}
	}

	for _, table := range domainTables {
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE deleted_at IS NOT NULL AND deleted_at < ?`, table),
			cutoff,
		); err != nil {
			return fmt.Errorf("purging tombstones in %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing purge: %w", err)
	}
	return nil
}

// tombstoneWatermarks finds, per user, the highest seq among
// tombstones about to be purged (across every domain table) — the
// value user_sync_counters.purged_before_seq needs to reach.
func tombstoneWatermarks(ctx context.Context, tx *sql.Tx, cutoff string) (map[string]int64, error) {
	watermarks := make(map[string]int64)

	for _, table := range domainTables {
		rows, err := tx.QueryContext(ctx,
			fmt.Sprintf(`SELECT user_id, MAX(seq) FROM %s WHERE deleted_at IS NOT NULL AND deleted_at < ? GROUP BY user_id`, table),
			cutoff,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tombstones in %s: %w", table, err)
		}

		scanErr := func() error {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var userID string
				var seq int64
				if err := rows.Scan(&userID, &seq); err != nil {
					return fmt.Errorf("scanning tombstone watermark in %s: %w", table, err)
				}
				if seq > watermarks[userID] {
					watermarks[userID] = seq
				}
			}
			return rows.Err()
		}()
		if scanErr != nil {
			return nil, scanErr
		}
	}

	return watermarks, nil
}

// PurgedBeforeSeq returns the seq below which userID's cursor is no
// longer guaranteed complete — some tombstones with a seq at or above
// this value were physically purged. 0 means nothing has been purged
// for this user yet.
func PurgedBeforeSeq(ctx context.Context, database *sql.DB, userID string) (int64, error) {
	var seq int64
	err := database.QueryRowContext(ctx,
		`SELECT purged_before_seq FROM user_sync_counters WHERE user_id = ?`, userID,
	).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("checking purge watermark: %w", err)
	}
	return seq, nil
}
