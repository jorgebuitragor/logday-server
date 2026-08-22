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
			updated_at = excluded.updated_at,
			deleted_at = NULL
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

// softDelete marks a task as deleted, provided it belongs to userID,
// returning the seq assigned to the tombstone (for the realtime
// notify call in the handler).
func (s *store) softDelete(ctx context.Context, id, userID string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID string
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM tasks WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&existingUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errNotFound
		}
		return 0, fmt.Errorf("checking task: %w", err)
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
		`UPDATE tasks SET deleted_at = ?, updated_at = ?, seq = ? WHERE id = ?`,
		now, now, seq, id,
	); err != nil {
		return 0, fmt.Errorf("deleting task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing task delete: %w", err)
	}
	return seq, nil
}

// Patch carries the fields present in a PATCH request — see
// specs/lww-por-campo. A field with Set == false is left untouched.
type Patch struct {
	Title       db.Field[string]
	TaskCode    db.Field[*string]
	Status      db.Field[string]
	Tags        db.Field[[]string]
	Project     db.Field[string]
	Created     db.Field[string]
	CompletedAt db.Field[*string]
	Due         db.Field[*string]
	Content     db.Field[string]
}

// patchTask merges patch into task id field by field, each field
// resolved independently against field_updated_at (LWW per field, not
// per row — see specs/lww-por-campo). Always returns the resulting row,
// even if every field in patch lost the LWW: the server is the only
// party that decides, the caller (HTTP handler) just relays the
// result. changed reports whether anything actually applied, so the
// handler knows whether to bump a realtime notification.
func (s *store) patchTask(ctx context.Context, id, userID string, patch Patch, updatedAt time.Time) (t *Task, changed bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, user_id, title, task_code, status, tags, project, created, completed_at, due, content, seq, updated_at, deleted_at, field_updated_at
		FROM tasks WHERE id = ?
	`, id)
	if err != nil {
		return nil, false, fmt.Errorf("loading task for patch: %w", err)
	}
	current, fieldUpdatedAtRaw, err := scanTaskWithFieldTimestamps(rows)
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
	if patch.TaskCode.Set && ft.Wins("task_code", updatedAt) {
		current.TaskCode = patch.TaskCode.Value
		changed = true
	}
	if patch.Status.Set && ft.Wins("status", updatedAt) {
		current.Status = patch.Status.Value
		changed = true
	}
	if patch.Tags.Set && ft.Wins("tags", updatedAt) {
		tags := patch.Tags.Value
		if tags == nil {
			tags = []string{}
		}
		current.Tags = tags
		changed = true
	}
	if patch.Project.Set && ft.Wins("project", updatedAt) {
		current.Project = patch.Project.Value
		changed = true
	}
	if patch.Created.Set && ft.Wins("created", updatedAt) {
		current.Created = patch.Created.Value
		changed = true
	}
	if patch.CompletedAt.Set && ft.Wins("completed_at", updatedAt) {
		current.CompletedAt = patch.CompletedAt.Value
		changed = true
	}
	if patch.Due.Set && ft.Wins("due", updatedAt) {
		current.Due = patch.Due.Value
		changed = true
	}
	if patch.Content.Set && ft.Wins("content", updatedAt) {
		current.Content = patch.Content.Value
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

	tagsJSON, err := json.Marshal(current.Tags)
	if err != nil {
		return nil, false, fmt.Errorf("encoding tags: %w", err)
	}
	ftEncoded, err := ft.Encode()
	if err != nil {
		return nil, false, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE tasks SET
			title = ?, task_code = ?, status = ?, tags = ?, project = ?, created = ?,
			completed_at = ?, due = ?, content = ?, seq = ?, updated_at = ?, deleted_at = NULL,
			field_updated_at = ?
		WHERE id = ?
	`,
		current.Title, current.TaskCode, current.Status, string(tagsJSON), current.Project, current.Created,
		current.CompletedAt, current.Due, current.Content, current.Seq, formatTime(current.UpdatedAt),
		ftEncoded, id,
	)
	if err != nil {
		return nil, false, fmt.Errorf("patching task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("committing task patch: %w", err)
	}
	return &current, true, nil
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

// scanTaskWithFieldTimestamps reads the single row from a query that
// selects the same columns as scanTask plus a trailing
// field_updated_at, returning errNotFound if there's no such row —
// used by patchTask, which needs that extra column and a not-found
// signal that scanTask's callers don't.
func scanTaskWithFieldTimestamps(rows *sql.Rows) (Task, string, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Task{}, "", fmt.Errorf("querying task: %w", err)
		}
		return Task{}, "", errNotFound
	}

	var t Task
	var tags, updatedAt, fieldUpdatedAt string
	var deletedAt sql.NullString
	if err := rows.Scan(
		&t.ID, &t.UserID, &t.Title, &t.TaskCode, &t.Status, &tags, &t.Project,
		&t.Created, &t.CompletedAt, &t.Due, &t.Content, &t.Seq, &updatedAt, &deletedAt, &fieldUpdatedAt,
	); err != nil {
		return Task{}, "", fmt.Errorf("scanning task: %w", err)
	}

	if err := json.Unmarshal([]byte(tags), &t.Tags); err != nil {
		return Task{}, "", fmt.Errorf("decoding tags: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return Task{}, "", err
	}
	t.UpdatedAt = updated

	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return Task{}, "", err
		}
		t.DeletedAt = &d
	}

	return t, fieldUpdatedAt, nil
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
