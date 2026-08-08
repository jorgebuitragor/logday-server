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
