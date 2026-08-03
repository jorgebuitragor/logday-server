package task

import (
	"context"
	"database/sql"
	"encoding/json"
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

// upsertTask creates t or, if a task with t.ID already exists, updates
// it — provided t belongs to the same user and t.UpdatedAt is newer
// than what's stored (LWW by whole row, see specs/sync-incremental).
func (s *store) upsertTask(ctx context.Context, t *Task) (*Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID, existingUpdatedAt string
	err = tx.QueryRowContext(ctx, `SELECT user_id, updated_at FROM tasks WHERE id = ?`, t.ID).
		Scan(&existingUserID, &existingUpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// New row, nothing to check.
	case err != nil:
		return nil, fmt.Errorf("checking existing task: %w", err)
	default:
		if existingUserID != t.UserID {
			return nil, errForbidden
		}
		existing, perr := parseTime(existingUpdatedAt)
		if perr != nil {
			return nil, perr
		}
		if !t.UpdatedAt.After(existing) {
			return nil, errConflict
		}
	}

	seq, err := db.NextSeq(ctx, tx, t.UserID)
	if err != nil {
		return nil, err
	}
	t.Seq = seq

	tagsJSON, err := json.Marshal(t.Tags)
	if err != nil {
		return nil, fmt.Errorf("encoding tags: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tasks (id, user_id, title, task_code, status, tags, project, created, completed_at, due, content, seq, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			task_code = excluded.task_code,
			status = excluded.status,
			tags = excluded.tags,
			project = excluded.project,
			created = excluded.created,
			completed_at = excluded.completed_at,
			due = excluded.due,
			content = excluded.content,
			seq = excluded.seq,
			updated_at = excluded.updated_at
	`,
		t.ID, t.UserID, t.Title, t.TaskCode, t.Status, string(tagsJSON), t.Project,
		t.Created, t.CompletedAt, t.Due, t.Content, t.Seq, formatTime(t.UpdatedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("upserting task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing task upsert: %w", err)
	}
	return t, nil
}

// softDelete marks a task as deleted, provided it belongs to userID.
func (s *store) softDelete(ctx context.Context, id, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID string
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM tasks WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&existingUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNotFound
		}
		return fmt.Errorf("checking task: %w", err)
	}
	if existingUserID != userID {
		return errForbidden
	}

	seq, err := db.NextSeq(ctx, tx, userID)
	if err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())

	if _, err := tx.ExecContext(ctx,
		`UPDATE tasks SET deleted_at = ?, updated_at = ?, seq = ? WHERE id = ?`,
		now, now, seq, id,
	); err != nil {
		return fmt.Errorf("deleting task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing task delete: %w", err)
	}
	return nil
}

func (s *store) listTasks(ctx context.Context, userID string) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, title, task_code, status, tags, project, created, completed_at, due, content, seq, updated_at, deleted_at
		FROM tasks WHERE user_id = ? AND deleted_at IS NULL ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tasks: %w", err)
	}
	return tasks, nil
}

// ChangesSince returns every task belonging to userID with seq > since
// (including soft-deleted ones, as tombstones), ordered by seq — the
// feed internal/sync merges into the unified GET /sync/changes
// response. Takes a raw *sql.DB instead of going through the
// unexported store type, so sync doesn't need to name it.
func ChangesSince(ctx context.Context, sqlDB *sql.DB, userID string, since int64) ([]Task, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT id, user_id, title, task_code, status, tags, project, created, completed_at, due, content, seq, updated_at, deleted_at
		FROM tasks WHERE user_id = ? AND seq > ? ORDER BY seq ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("querying task changes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating task changes: %w", err)
	}
	return tasks, nil
}

func scanTask(row *sql.Rows) (Task, error) {
	var t Task
	var tags, updatedAt string
	var deletedAt sql.NullString
	if err := row.Scan(
		&t.ID, &t.UserID, &t.Title, &t.TaskCode, &t.Status, &tags, &t.Project,
		&t.Created, &t.CompletedAt, &t.Due, &t.Content, &t.Seq, &updatedAt, &deletedAt,
	); err != nil {
		return Task{}, fmt.Errorf("scanning task: %w", err)
	}

	if err := json.Unmarshal([]byte(tags), &t.Tags); err != nil {
		return Task{}, fmt.Errorf("decoding tags: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return Task{}, err
	}
	t.UpdatedAt = updated

	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return Task{}, err
		}
		t.DeletedAt = &d
	}

	return t, nil
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
