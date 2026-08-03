package note

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

// upsertNote creates n or, if a note with n.ID already exists, updates
// it — provided n belongs to the same user and n.UpdatedAt is newer
// than what's stored (LWW by whole row, see specs/sync-incremental).
func (s *store) upsertNote(ctx context.Context, n *Note) (*Note, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID, existingUpdatedAt string
	err = tx.QueryRowContext(ctx, `SELECT user_id, updated_at FROM notes WHERE id = ?`, n.ID).
		Scan(&existingUserID, &existingUpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// New row, nothing to check.
	case err != nil:
		return nil, fmt.Errorf("checking existing note: %w", err)
	default:
		if existingUserID != n.UserID {
			return nil, errForbidden
		}
		existing, perr := parseTime(existingUpdatedAt)
		if perr != nil {
			return nil, perr
		}
		if !n.UpdatedAt.After(existing) {
			return nil, errConflict
		}
	}

	seq, err := db.NextSeq(ctx, tx, n.UserID)
	if err != nil {
		return nil, err
	}
	n.Seq = seq

	tagsJSON, err := json.Marshal(n.Tags)
	if err != nil {
		return nil, fmt.Errorf("encoding tags: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO notes (id, user_id, title, folder, tags, created, updated, pinned, content, seq, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			folder = excluded.folder,
			tags = excluded.tags,
			created = excluded.created,
			updated = excluded.updated,
			pinned = excluded.pinned,
			content = excluded.content,
			seq = excluded.seq,
			updated_at = excluded.updated_at
	`,
		n.ID, n.UserID, n.Title, n.Folder, string(tagsJSON), n.Created, n.Updated,
		n.Pinned, n.Content, n.Seq, formatTime(n.UpdatedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("upserting note: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing note upsert: %w", err)
	}
	return n, nil
}

// softDelete marks a note as deleted, provided it belongs to userID.
func (s *store) softDelete(ctx context.Context, id, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID string
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM notes WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&existingUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNotFound
		}
		return fmt.Errorf("checking note: %w", err)
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
		`UPDATE notes SET deleted_at = ?, updated_at = ?, seq = ? WHERE id = ?`,
		now, now, seq, id,
	); err != nil {
		return fmt.Errorf("deleting note: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing note delete: %w", err)
	}
	return nil
}

func (s *store) listNotes(ctx context.Context, userID string) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, title, folder, tags, created, updated, pinned, content, seq, updated_at, deleted_at
		FROM notes WHERE user_id = ? AND deleted_at IS NULL ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notes []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating notes: %w", err)
	}
	return notes, nil
}

// ChangesSince returns every note belonging to userID with seq > since
// (including soft-deleted ones, as tombstones), ordered by seq — the
// feed internal/sync merges into the unified GET /sync/changes
// response. Takes a raw *sql.DB instead of going through the
// unexported store type, so sync doesn't need to name it.
func ChangesSince(ctx context.Context, sqlDB *sql.DB, userID string, since int64) ([]Note, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT id, user_id, title, folder, tags, created, updated, pinned, content, seq, updated_at, deleted_at
		FROM notes WHERE user_id = ? AND seq > ? ORDER BY seq ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("querying note changes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notes []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating note changes: %w", err)
	}
	return notes, nil
}

func scanNote(row *sql.Rows) (Note, error) {
	var n Note
	var tags, updatedAt string
	var deletedAt sql.NullString
	if err := row.Scan(
		&n.ID, &n.UserID, &n.Title, &n.Folder, &tags, &n.Created, &n.Updated,
		&n.Pinned, &n.Content, &n.Seq, &updatedAt, &deletedAt,
	); err != nil {
		return Note{}, fmt.Errorf("scanning note: %w", err)
	}

	if err := json.Unmarshal([]byte(tags), &n.Tags); err != nil {
		return Note{}, fmt.Errorf("decoding tags: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return Note{}, err
	}
	n.UpdatedAt = updated

	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return Note{}, err
		}
		n.DeletedAt = &d
	}

	return n, nil
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
