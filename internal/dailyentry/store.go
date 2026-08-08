package dailyentry

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/crdt"
	"github.com/jorgebuitragor/logday-server/internal/db"
)

var errNotFound = errors.New("not found")

type store struct {
	db *sql.DB
}

// NewStore wires up the SQL-backed store used by NewHandler.
func NewStore(sqlDB *sql.DB) *store {
	return &store{db: sqlDB}
}

// applyContentUpdate merges a client's CRDT update into the entry at
// (userID, date), creating the row if it doesn't exist yet. Unlike a
// plain LWW upsert, this never rejects on staleness — CRDT updates
// commute and are idempotent, so there's nothing to reject (see
// specs/arquitectura-inicial, "Resolución de conflictos").
func (s *store) applyContentUpdate(ctx context.Context, userID, date string, update []byte, updatedAt time.Time) (*Entry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingCRDT []byte
	err = tx.QueryRowContext(ctx,
		`SELECT content_crdt FROM daily_entries WHERE user_id = ? AND date = ?`,
		userID, date,
	).Scan(&existingCRDT)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("checking existing daily entry: %w", err)
	}

	newState, _, err := crdt.ApplyTextUpdate(existingCRDT, update)
	if err != nil {
		return nil, fmt.Errorf("merging content update: %w", err)
	}

	seq, err := db.NextSeq(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO daily_entries (user_id, date, content_crdt, seq, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET
			content_crdt = excluded.content_crdt,
			seq = excluded.seq,
			updated_at = excluded.updated_at,
			deleted_at = NULL
	`, userID, date, newState, seq, formatTime(updatedAt)); err != nil {
		return nil, fmt.Errorf("upserting daily entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing content update: %w", err)
	}

	// Fetch fresh rather than assembling from parts on hand: keeps the
	// decoding of content_crdt (into Content/ContentState) in exactly
	// one place (scanEntry), instead of duplicating it here.
	return s.getEntry(ctx, userID, date)
}

func (s *store) getEntry(ctx context.Context, userID, date string) (*Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, date, content_crdt, seq, updated_at, deleted_at
		FROM daily_entries WHERE user_id = ? AND date = ?
	`, userID, date)
	if err != nil {
		return nil, fmt.Errorf("querying daily entry: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("querying daily entry: %w", err)
		}
		return nil, errNotFound
	}
	e, err := scanEntry(rows)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *store) softDelete(ctx context.Context, userID, date string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM daily_entries WHERE user_id = ? AND date = ? AND deleted_at IS NULL`,
		userID, date,
	).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errNotFound
		}
		return 0, fmt.Errorf("checking daily entry: %w", err)
	}

	seq, err := db.NextSeq(ctx, tx, userID)
	if err != nil {
		return 0, err
	}
	now := formatTime(time.Now().UTC())

	if _, err := tx.ExecContext(ctx,
		`UPDATE daily_entries SET deleted_at = ?, updated_at = ?, seq = ? WHERE user_id = ? AND date = ?`,
		now, now, seq, userID, date,
	); err != nil {
		return 0, fmt.Errorf("deleting daily entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing daily entry delete: %w", err)
	}
	return seq, nil
}

func (s *store) listEntries(ctx context.Context, userID string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, date, content_crdt, seq, updated_at, deleted_at
		FROM daily_entries WHERE user_id = ? AND deleted_at IS NULL ORDER BY date DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying daily entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating daily entries: %w", err)
	}
	return entries, nil
}

// ChangesSince returns every daily entry belonging to userID with
// seq > since (including soft-deleted ones, as tombstones), ordered
// by seq — merged by internal/sync into GET /sync/changes.
func ChangesSince(ctx context.Context, sqlDB *sql.DB, userID string, since int64) ([]Entry, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT user_id, date, content_crdt, seq, updated_at, deleted_at
		FROM daily_entries WHERE user_id = ? AND seq > ? ORDER BY seq ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("querying daily entry changes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating daily entry changes: %w", err)
	}
	return entries, nil
}

func scanEntry(row *sql.Rows) (Entry, error) {
	var e Entry
	var updatedAt string
	var deletedAt sql.NullString
	if err := row.Scan(&e.UserID, &e.Date, &e.ContentCRDT, &e.Seq, &updatedAt, &deletedAt); err != nil {
		return Entry{}, fmt.Errorf("scanning daily entry: %w", err)
	}

	text, err := crdt.Text(e.ContentCRDT)
	if err != nil {
		return Entry{}, fmt.Errorf("decoding content: %w", err)
	}
	e.Content = text
	if len(e.ContentCRDT) > 0 {
		e.ContentState = base64.StdEncoding.EncodeToString(e.ContentCRDT)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return Entry{}, err
	}
	e.UpdatedAt = updated

	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return Entry{}, err
		}
		e.DeletedAt = &d
	}

	return e, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing time %q: %w", s, err)
	}
	return t, nil
}
