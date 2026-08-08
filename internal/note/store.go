package note

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/crdt"
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
// than what's stored (LWW by row, see specs/sync-incremental). Never
// touches content_crdt: content is written exclusively through
// applyContentUpdate, which merges instead of rejecting on staleness.
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
		INSERT INTO notes (id, user_id, title, folder, tags, created, updated, pinned, seq, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			folder = excluded.folder,
			tags = excluded.tags,
			created = excluded.created,
			updated = excluded.updated,
			pinned = excluded.pinned,
			seq = excluded.seq,
			updated_at = excluded.updated_at,
			deleted_at = NULL
	`,
		n.ID, n.UserID, n.Title, n.Folder, string(tagsJSON), n.Created, n.Updated,
		n.Pinned, n.Seq, formatTime(n.UpdatedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("upserting note: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing note upsert: %w", err)
	}

	// This upsert never touches content_crdt (see applyContentUpdate);
	// fetch fresh so the response reflects whatever content already
	// exists instead of returning it blank.
	return s.getNote(ctx, n.ID)
}

// applyContentUpdate merges a client's CRDT update into the note's
// stored content, provided the note belongs to userID — unlike
// upsertNote, this never rejects on staleness: CRDT updates commute
// and are idempotent, so there's nothing to reject (see
// specs/arquitectura-inicial, "Resolución de conflictos").
func (s *store) applyContentUpdate(ctx context.Context, id, userID string, update []byte, updatedAt time.Time) (*Note, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID string
	var existingCRDT []byte
	err = tx.QueryRowContext(ctx, `SELECT user_id, content_crdt FROM notes WHERE id = ?`, id).
		Scan(&existingUserID, &existingCRDT)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errNotFound
		}
		return nil, fmt.Errorf("checking existing note: %w", err)
	}
	if existingUserID != userID {
		return nil, errForbidden
	}

	newState, _, err := crdt.ApplyTextUpdate(existingCRDT, update)
	if err != nil {
		return nil, fmt.Errorf("merging content update: %w", err)
	}

	seq, err := db.NextSeq(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE notes SET content_crdt = ?, seq = ?, updated_at = ? WHERE id = ?`,
		newState, seq, formatTime(updatedAt), id,
	); err != nil {
		return nil, fmt.Errorf("storing merged content: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing content update: %w", err)
	}

	// Fetch fresh rather than assembling from parts on hand: keeps the
	// decoding of content_crdt (into Content/ContentState) in exactly
	// one place (scanNote), instead of duplicating it here.
	return s.getNote(ctx, id)
}

func (s *store) getNote(ctx context.Context, id string) (*Note, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, title, folder, tags, created, updated, pinned, content_crdt, seq, updated_at, deleted_at
		FROM notes WHERE id = ?
	`, id)
	if err != nil {
		return nil, fmt.Errorf("querying note: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("querying note: %w", err)
		}
		return nil, errNotFound
	}
	n, err := scanNote(rows)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// softDelete marks a note as deleted, provided it belongs to userID,
// returning the seq assigned to the tombstone (for the realtime
// notify call in the handler).
func (s *store) softDelete(ctx context.Context, id, userID string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID string
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM notes WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&existingUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errNotFound
		}
		return 0, fmt.Errorf("checking note: %w", err)
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
		`UPDATE notes SET deleted_at = ?, updated_at = ?, seq = ? WHERE id = ?`,
		now, now, seq, id,
	); err != nil {
		return 0, fmt.Errorf("deleting note: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing note delete: %w", err)
	}
	return seq, nil
}

func (s *store) listNotes(ctx context.Context, userID string) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, title, folder, tags, created, updated, pinned, content_crdt, seq, updated_at, deleted_at
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
		SELECT id, user_id, title, folder, tags, created, updated, pinned, content_crdt, seq, updated_at, deleted_at
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
		&n.Pinned, &n.ContentCRDT, &n.Seq, &updatedAt, &deletedAt,
	); err != nil {
		return Note{}, fmt.Errorf("scanning note: %w", err)
	}

	if err := json.Unmarshal([]byte(tags), &n.Tags); err != nil {
		return Note{}, fmt.Errorf("decoding tags: %w", err)
	}

	text, err := crdt.Text(n.ContentCRDT)
	if err != nil {
		return Note{}, fmt.Errorf("decoding content: %w", err)
	}
	n.Content = text
	if len(n.ContentCRDT) > 0 {
		n.ContentState = base64.StdEncoding.EncodeToString(n.ContentCRDT)
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
