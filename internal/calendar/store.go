package calendar

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

func (s *store) upsertEvent(ctx context.Context, e *Event) (*Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID, existingUpdatedAt string
	err = tx.QueryRowContext(ctx, `SELECT user_id, updated_at FROM calendar_events WHERE id = ?`, e.ID).
		Scan(&existingUserID, &existingUpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// New row, nothing to check.
	case err != nil:
		return nil, fmt.Errorf("checking existing calendar event: %w", err)
	default:
		if existingUserID != e.UserID {
			return nil, errForbidden
		}
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
		INSERT INTO calendar_events (id, user_id, title, date, time, description, color, reminder_minutes, repeat, seq, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			date = excluded.date,
			time = excluded.time,
			description = excluded.description,
			color = excluded.color,
			reminder_minutes = excluded.reminder_minutes,
			repeat = excluded.repeat,
			seq = excluded.seq,
			updated_at = excluded.updated_at,
			deleted_at = NULL
	`,
		e.ID, e.UserID, e.Title, e.Date, e.Time, e.Description, e.Color,
		e.ReminderMinutes, e.Repeat, e.Seq, formatTime(e.UpdatedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("upserting calendar event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing calendar event upsert: %w", err)
	}
	return e, nil
}

func (s *store) softDelete(ctx context.Context, id, userID string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID string
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM calendar_events WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&existingUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errNotFound
		}
		return 0, fmt.Errorf("checking calendar event: %w", err)
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
		`UPDATE calendar_events SET deleted_at = ?, updated_at = ?, seq = ? WHERE id = ?`,
		now, now, seq, id,
	); err != nil {
		return 0, fmt.Errorf("deleting calendar event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing calendar event delete: %w", err)
	}
	return seq, nil
}

// Patch carries the fields present in a PATCH request against a
// calendar event — see specs/lww-por-campo.
type Patch struct {
	Title           db.Field[string]
	Date            db.Field[string]
	Time            db.Field[string]
	Description     db.Field[string]
	Color           db.Field[string]
	ReminderMinutes db.Field[int]
	Repeat          db.Field[string]
}

// patchEvent merges patch into calendar event id field by field, each
// resolved independently against field_updated_at (LWW per field —
// see specs/lww-por-campo). Always returns the resulting row, even if
// every field in patch lost the LWW. changed reports whether anything
// actually applied.
func (s *store) patchEvent(ctx context.Context, id, userID string, patch Patch, updatedAt time.Time) (e *Event, changed bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, user_id, title, date, time, description, color, reminder_minutes, repeat, seq, updated_at, deleted_at, field_updated_at
		FROM calendar_events WHERE id = ?
	`, id)
	if err != nil {
		return nil, false, fmt.Errorf("loading calendar event for patch: %w", err)
	}
	current, fieldUpdatedAtRaw, err := scanEventWithFieldTimestamps(rows)
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

	if patch.Title.Set && ft.Wins("title", updatedAt) {
		current.Title = patch.Title.Value
		changed = true
	}
	if patch.Date.Set && ft.Wins("date", updatedAt) {
		current.Date = patch.Date.Value
		changed = true
	}
	if patch.Time.Set && ft.Wins("time", updatedAt) {
		current.Time = patch.Time.Value
		changed = true
	}
	if patch.Description.Set && ft.Wins("description", updatedAt) {
		current.Description = patch.Description.Value
		changed = true
	}
	if patch.Color.Set && ft.Wins("color", updatedAt) {
		current.Color = patch.Color.Value
		changed = true
	}
	if patch.ReminderMinutes.Set && ft.Wins("reminder_minutes", updatedAt) {
		current.ReminderMinutes = patch.ReminderMinutes.Value
		changed = true
	}
	if patch.Repeat.Set && ft.Wins("repeat", updatedAt) {
		current.Repeat = patch.Repeat.Value
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
		UPDATE calendar_events SET
			title = ?, date = ?, time = ?, description = ?, color = ?, reminder_minutes = ?, repeat = ?,
			seq = ?, updated_at = ?, deleted_at = NULL, field_updated_at = ?
		WHERE id = ?
	`,
		current.Title, current.Date, current.Time, current.Description, current.Color,
		current.ReminderMinutes, current.Repeat, current.Seq, formatTime(current.UpdatedAt), ftEncoded, id,
	)
	if err != nil {
		return nil, false, fmt.Errorf("patching calendar event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("committing calendar event patch: %w", err)
	}
	return &current, true, nil
}

func (s *store) listEvents(ctx context.Context, userID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, title, date, time, description, color, reminder_minutes, repeat, seq, updated_at, deleted_at
		FROM calendar_events WHERE user_id = ? AND deleted_at IS NULL ORDER BY date ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying calendar events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating calendar events: %w", err)
	}
	return events, nil
}

// ChangesSince returns every calendar event belonging to userID with
// seq > since (including soft-deleted ones, as tombstones), ordered
// by seq — merged by internal/sync into GET /sync/changes.
func ChangesSince(ctx context.Context, sqlDB *sql.DB, userID string, since int64) ([]Event, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT id, user_id, title, date, time, description, color, reminder_minutes, repeat, seq, updated_at, deleted_at
		FROM calendar_events WHERE user_id = ? AND seq > ? ORDER BY seq ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("querying calendar event changes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating calendar event changes: %w", err)
	}
	return events, nil
}

func scanEvent(row *sql.Rows) (Event, error) {
	var e Event
	var updatedAt string
	var deletedAt sql.NullString
	if err := row.Scan(
		&e.ID, &e.UserID, &e.Title, &e.Date, &e.Time, &e.Description,
		&e.Color, &e.ReminderMinutes, &e.Repeat, &e.Seq, &updatedAt, &deletedAt,
	); err != nil {
		return Event{}, fmt.Errorf("scanning calendar event: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return Event{}, err
	}
	e.UpdatedAt = updated

	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return Event{}, err
		}
		e.DeletedAt = &d
	}

	return e, nil
}

// scanEventWithFieldTimestamps reads the single row from a query
// selecting the same columns as scanEvent plus a trailing
// field_updated_at, returning errNotFound if there's no such row —
// used by patchEvent.
func scanEventWithFieldTimestamps(rows *sql.Rows) (Event, string, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Event{}, "", fmt.Errorf("querying calendar event: %w", err)
		}
		return Event{}, "", errNotFound
	}

	var e Event
	var updatedAt, fieldUpdatedAt string
	var deletedAt sql.NullString
	if err := rows.Scan(
		&e.ID, &e.UserID, &e.Title, &e.Date, &e.Time, &e.Description,
		&e.Color, &e.ReminderMinutes, &e.Repeat, &e.Seq, &updatedAt, &deletedAt, &fieldUpdatedAt,
	); err != nil {
		return Event{}, "", fmt.Errorf("scanning calendar event: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return Event{}, "", err
	}
	e.UpdatedAt = updated

	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return Event{}, "", err
		}
		e.DeletedAt = &d
	}

	return e, fieldUpdatedAt, nil
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
