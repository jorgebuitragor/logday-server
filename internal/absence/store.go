package absence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/db"
)

var (
	errNotFound  = errors.New("not found")
	errForbidden = errors.New("forbidden")
	errConflict  = errors.New("conflict")
)

type store struct {
	db *sql.DB
}

// NewStore wires up the SQL-backed store used by NewHandler.
func NewStore(sqlDB *sql.DB) *store {
	return &store{db: sqlDB}
}

func (s *store) upsertDay(ctx context.Context, d *Day) (*Day, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID, existingUpdatedAt string
	err = tx.QueryRowContext(ctx, `SELECT user_id, updated_at FROM absence_days WHERE id = ?`, d.ID).
		Scan(&existingUserID, &existingUpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// New row, nothing to check.
	case err != nil:
		return nil, fmt.Errorf("checking existing absence day: %w", err)
	default:
		if existingUserID != d.UserID {
			return nil, errForbidden
		}
		existing, perr := parseTime(existingUpdatedAt)
		if perr != nil {
			return nil, perr
		}
		if !d.UpdatedAt.After(existing) {
			return nil, errConflict
		}
	}

	seq, err := db.NextSeq(ctx, tx, d.UserID)
	if err != nil {
		return nil, err
	}
	d.Seq = seq

	_, err = tx.ExecContext(ctx, `
		INSERT INTO absence_days (id, user_id, date, type, note, seq, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			date = excluded.date,
			type = excluded.type,
			note = excluded.note,
			seq = excluded.seq,
			updated_at = excluded.updated_at,
			deleted_at = NULL
	`, d.ID, d.UserID, d.Date, d.Type, d.Note, d.Seq, formatTime(d.UpdatedAt))
	if err != nil {
		return nil, fmt.Errorf("upserting absence day: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing absence day upsert: %w", err)
	}
	return d, nil
}

func (s *store) softDelete(ctx context.Context, id, userID string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID string
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM absence_days WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&existingUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errNotFound
		}
		return 0, fmt.Errorf("checking absence day: %w", err)
	}
	if existingUserID != userID {
		return 0, errForbidden
	}

	seq, err := db.NextSeq(ctx, tx, userID)
	if err != nil {
		return 0, err
	}
	now := formatTime(time.Now().UTC())

	if _, err := tx.ExecContext(ctx,
		`UPDATE absence_days SET deleted_at = ?, updated_at = ?, seq = ? WHERE id = ?`,
		now, now, seq, id,
	); err != nil {
		return 0, fmt.Errorf("deleting absence day: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing absence day delete: %w", err)
	}
	return seq, nil
}

// Patch carries the fields present in a PATCH request against an
// absence day — see specs/lww-por-campo.
type Patch struct {
	Date db.Field[string]
	Type db.Field[string]
	Note db.Field[*string]
}

// patchDay merges patch into absence day id field by field, each
// resolved independently against field_updated_at (LWW per field —
// see specs/lww-por-campo). Always returns the resulting row, even if
// every field in patch lost the LWW. changed reports whether anything
// actually applied.
func (s *store) patchDay(ctx context.Context, id, userID string, patch Patch, updatedAt time.Time) (d *Day, changed bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, user_id, date, type, note, seq, updated_at, deleted_at, field_updated_at
		FROM absence_days WHERE id = ?
	`, id)
	if err != nil {
		return nil, false, fmt.Errorf("loading absence day for patch: %w", err)
	}
	current, fieldUpdatedAtRaw, err := scanDayWithFieldTimestamps(rows)
	_ = rows.Close()
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, false, errNotFound
		}
		return nil, false, err
	}
	if current.UserID != userID {
		return nil, false, errForbidden
	}

	ft, err := db.ParseFieldTimestamps(fieldUpdatedAtRaw)
	if err != nil {
		return nil, false, err
	}

	if patch.Date.Set && ft.Wins("date", updatedAt) {
		current.Date = patch.Date.Value
		changed = true
	}
	if patch.Type.Set && ft.Wins("type", updatedAt) {
		current.Type = patch.Type.Value
		changed = true
	}
	if patch.Note.Set && ft.Wins("note", updatedAt) {
		current.Note = patch.Note.Value
		changed = true
	}

	if !changed {
		return &current, false, nil
	}

	seq, err := db.NextSeq(ctx, tx, userID)
	if err != nil {
		return nil, false, err
	}
	current.Seq = seq
	current.UpdatedAt = time.Now().UTC()
	current.DeletedAt = nil

	ftEncoded, err := ft.Encode()
	if err != nil {
		return nil, false, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE absence_days SET
			date = ?, type = ?, note = ?, seq = ?, updated_at = ?, deleted_at = NULL, field_updated_at = ?
		WHERE id = ?
	`, current.Date, current.Type, current.Note, current.Seq, formatTime(current.UpdatedAt), ftEncoded, id)
	if err != nil {
		return nil, false, fmt.Errorf("patching absence day: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("committing absence day patch: %w", err)
	}
	return &current, true, nil
}

func (s *store) listDays(ctx context.Context, userID string) ([]Day, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, date, type, note, seq, updated_at, deleted_at
		FROM absence_days WHERE user_id = ? AND deleted_at IS NULL ORDER BY date DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying absence days: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var days []Day
	for rows.Next() {
		d, err := scanDay(rows)
		if err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating absence days: %w", err)
	}
	return days, nil
}

// ChangesSince returns every absence day belonging to userID with
// seq > since (including soft-deleted ones, as tombstones), ordered
// by seq — merged by internal/sync into GET /sync/changes.
func ChangesSince(ctx context.Context, sqlDB *sql.DB, userID string, since int64) ([]Day, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT id, user_id, date, type, note, seq, updated_at, deleted_at
		FROM absence_days WHERE user_id = ? AND seq > ? ORDER BY seq ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("querying absence day changes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var days []Day
	for rows.Next() {
		d, err := scanDay(rows)
		if err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating absence day changes: %w", err)
	}
	return days, nil
}

func scanDay(row *sql.Rows) (Day, error) {
	var d Day
	var updatedAt string
	var deletedAt sql.NullString
	if err := row.Scan(&d.ID, &d.UserID, &d.Date, &d.Type, &d.Note, &d.Seq, &updatedAt, &deletedAt); err != nil {
		return Day{}, fmt.Errorf("scanning absence day: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return Day{}, err
	}
	d.UpdatedAt = updated

	if deletedAt.Valid {
		dt, err := parseTime(deletedAt.String)
		if err != nil {
			return Day{}, err
		}
		d.DeletedAt = &dt
	}

	return d, nil
}

// scanDayWithFieldTimestamps reads the single row from a query
// selecting the same columns as scanDay plus a trailing
// field_updated_at, returning errNotFound if there's no such row —
// used by patchDay.
func scanDayWithFieldTimestamps(rows *sql.Rows) (Day, string, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Day{}, "", fmt.Errorf("querying absence day: %w", err)
		}
		return Day{}, "", errNotFound
	}

	var d Day
	var updatedAt, fieldUpdatedAt string
	var deletedAt sql.NullString
	if err := rows.Scan(&d.ID, &d.UserID, &d.Date, &d.Type, &d.Note, &d.Seq, &updatedAt, &deletedAt, &fieldUpdatedAt); err != nil {
		return Day{}, "", fmt.Errorf("scanning absence day: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return Day{}, "", err
	}
	d.UpdatedAt = updated

	if deletedAt.Valid {
		dt, err := parseTime(deletedAt.String)
		if err != nil {
			return Day{}, "", err
		}
		d.DeletedAt = &dt
	}

	return d, fieldUpdatedAt, nil
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
