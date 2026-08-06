package dailyentry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/db"
)

var (
	errNotFound = errors.New("not found")
	errConflict = errors.New("conflict")
)

type store struct {
	db *sql.DB
}

// NewStore wires up the SQL-backed store used by NewHandler.
func NewStore(sqlDB *sql.DB) *store {
	return &store{db: sqlDB}
}

func (s *store) upsertEntry(ctx context.Context, e *Entry) (*Entry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUpdatedAt string
	err = tx.QueryRowContext(ctx,
		`SELECT updated_at FROM daily_entries WHERE user_id = ? AND date = ?`,
		e.UserID, e.Date,
	).Scan(&existingUpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// New row, nothing to check. No cross-user ownership check
		// needed either — the row is keyed by user_id itself.
	case err != nil:
		return nil, fmt.Errorf("checking existing daily entry: %w", err)
	default:
		existing, perr := parseTime(existingUpdatedAt)
		if perr != nil {
			return nil, perr
		}
		if !e.UpdatedAt.After(existing) {
			return nil, errConflict
		}
	}

	seq, err := db.NextSeq(ctx, tx, e.UserID)
	if err != nil {
		return nil, err
	}
	e.Seq = seq

	_, err = tx.ExecContext(ctx, `
		INSERT INTO daily_entries (user_id, date, content, seq, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET
			content = excluded.content,
			seq = excluded.seq,
			updated_at = excluded.updated_at
	`, e.UserID, e.Date, e.Content, e.Seq, formatTime(e.UpdatedAt))
	if err != nil {
		return nil, fmt.Errorf("upserting daily entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing daily entry upsert: %w", err)
	}
	return e, nil
}

func (s *store) softDelete(ctx context.Context, userID, date string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM daily_entries WHERE user_id = ? AND date = ? AND deleted_at IS NULL`,
		userID, date,
	).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNotFound
		}
		return fmt.Errorf("checking daily entry: %w", err)
	}

	seq, err := db.NextSeq(ctx, tx, userID)
	if err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())

	if _, err := tx.ExecContext(ctx,
		`UPDATE daily_entries SET deleted_at = ?, updated_at = ?, seq = ? WHERE user_id = ? AND date = ?`,
		now, now, seq, userID, date,
	); err != nil {
		return fmt.Errorf("deleting daily entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing daily entry delete: %w", err)
	}
	return nil
}

func (s *store) listEntries(ctx context.Context, userID string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, date, content, seq, updated_at, deleted_at
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
		SELECT user_id, date, content, seq, updated_at, deleted_at
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
	if err := row.Scan(&e.UserID, &e.Date, &e.Content, &e.Seq, &updatedAt, &deletedAt); err != nil {
		return Entry{}, fmt.Errorf("scanning daily entry: %w", err)
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
