package overtime

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

// --- Entry ---

func (s *store) upsertEntry(ctx context.Context, e *Entry) (*Entry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID, existingUpdatedAt string
	err = tx.QueryRowContext(ctx, `SELECT user_id, updated_at FROM overtime_entries WHERE id = ?`, e.ID).
		Scan(&existingUserID, &existingUpdatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// New row, nothing to check.
	case err != nil:
		return nil, fmt.Errorf("checking existing overtime entry: %w", err)
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
		INSERT INTO overtime_entries (
			id, user_id, fecha, solicitada_por, actividad, observaciones,
			hora_inicio, hora_final, total_horas, extras_diurnas, extras_nocturnas,
			extras_diurnas_festivas, extras_nocturnas_festivas, seq, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			fecha = excluded.fecha,
			solicitada_por = excluded.solicitada_por,
			actividad = excluded.actividad,
			observaciones = excluded.observaciones,
			hora_inicio = excluded.hora_inicio,
			hora_final = excluded.hora_final,
			total_horas = excluded.total_horas,
			extras_diurnas = excluded.extras_diurnas,
			extras_nocturnas = excluded.extras_nocturnas,
			extras_diurnas_festivas = excluded.extras_diurnas_festivas,
			extras_nocturnas_festivas = excluded.extras_nocturnas_festivas,
			seq = excluded.seq,
			updated_at = excluded.updated_at,
			deleted_at = NULL
	`,
		e.ID, e.UserID, e.Fecha, e.SolicitadaPor, e.Actividad, e.Observaciones,
		e.HoraInicio, e.HoraFinal, e.TotalHoras, e.ExtrasDiurnas, e.ExtrasNocturnas,
		e.ExtrasDiurnasFestivas, e.ExtrasNocturnasFestivas, e.Seq, formatTime(e.UpdatedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("upserting overtime entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing overtime entry upsert: %w", err)
	}
	return e, nil
}

func (s *store) softDeleteEntry(ctx context.Context, id, userID string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingUserID string
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM overtime_entries WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&existingUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errNotFound
		}
		return 0, fmt.Errorf("checking overtime entry: %w", err)
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
		`UPDATE overtime_entries SET deleted_at = ?, updated_at = ?, seq = ? WHERE id = ?`,
		now, now, seq, id,
	); err != nil {
		return 0, fmt.Errorf("deleting overtime entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing overtime entry delete: %w", err)
	}
	return seq, nil
}

// EntryPatch carries the fields present in a PATCH request against an
// overtime entry — see specs/lww-por-campo.
type EntryPatch struct {
	Fecha                   db.Field[string]
	SolicitadaPor           db.Field[string]
	Actividad               db.Field[string]
	Observaciones           db.Field[string]
	HoraInicio              db.Field[string]
	HoraFinal               db.Field[string]
	TotalHoras              db.Field[float64]
	ExtrasDiurnas           db.Field[float64]
	ExtrasNocturnas         db.Field[float64]
	ExtrasDiurnasFestivas   db.Field[float64]
	ExtrasNocturnasFestivas db.Field[float64]
}

// patchEntry merges patch into overtime entry id field by field,
// each resolved independently against field_updated_at (LWW per
// field — see specs/lww-por-campo). Always returns the resulting row,
// even if every field in patch lost the LWW. changed reports whether
// anything actually applied.
func (s *store) patchEntry(ctx context.Context, id, userID string, patch EntryPatch, updatedAt time.Time) (e *Entry, changed bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, user_id, fecha, solicitada_por, actividad, observaciones,
			hora_inicio, hora_final, total_horas, extras_diurnas, extras_nocturnas,
			extras_diurnas_festivas, extras_nocturnas_festivas, seq, updated_at, deleted_at, field_updated_at
		FROM overtime_entries WHERE id = ?
	`, id)
	if err != nil {
		return nil, false, fmt.Errorf("loading overtime entry for patch: %w", err)
	}
	current, fieldUpdatedAtRaw, err := scanEntryWithFieldTimestamps(rows)
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

	if patch.Fecha.Set && ft.Wins("fecha", updatedAt) {
		current.Fecha = patch.Fecha.Value
		changed = true
	}
	if patch.SolicitadaPor.Set && ft.Wins("solicitada_por", updatedAt) {
		current.SolicitadaPor = patch.SolicitadaPor.Value
		changed = true
	}
	if patch.Actividad.Set && ft.Wins("actividad", updatedAt) {
		current.Actividad = patch.Actividad.Value
		changed = true
	}
	if patch.Observaciones.Set && ft.Wins("observaciones", updatedAt) {
		current.Observaciones = patch.Observaciones.Value
		changed = true
	}
	if patch.HoraInicio.Set && ft.Wins("hora_inicio", updatedAt) {
		current.HoraInicio = patch.HoraInicio.Value
		changed = true
	}
	if patch.HoraFinal.Set && ft.Wins("hora_final", updatedAt) {
		current.HoraFinal = patch.HoraFinal.Value
		changed = true
	}
	if patch.TotalHoras.Set && ft.Wins("total_horas", updatedAt) {
		current.TotalHoras = patch.TotalHoras.Value
		changed = true
	}
	if patch.ExtrasDiurnas.Set && ft.Wins("extras_diurnas", updatedAt) {
		current.ExtrasDiurnas = patch.ExtrasDiurnas.Value
		changed = true
	}
	if patch.ExtrasNocturnas.Set && ft.Wins("extras_nocturnas", updatedAt) {
		current.ExtrasNocturnas = patch.ExtrasNocturnas.Value
		changed = true
	}
	if patch.ExtrasDiurnasFestivas.Set && ft.Wins("extras_diurnas_festivas", updatedAt) {
		current.ExtrasDiurnasFestivas = patch.ExtrasDiurnasFestivas.Value
		changed = true
	}
	if patch.ExtrasNocturnasFestivas.Set && ft.Wins("extras_nocturnas_festivas", updatedAt) {
		current.ExtrasNocturnasFestivas = patch.ExtrasNocturnasFestivas.Value
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
		UPDATE overtime_entries SET
			fecha = ?, solicitada_por = ?, actividad = ?, observaciones = ?,
			hora_inicio = ?, hora_final = ?, total_horas = ?, extras_diurnas = ?,
			extras_nocturnas = ?, extras_diurnas_festivas = ?, extras_nocturnas_festivas = ?,
			seq = ?, updated_at = ?, deleted_at = NULL, field_updated_at = ?
		WHERE id = ?
	`,
		current.Fecha, current.SolicitadaPor, current.Actividad, current.Observaciones,
		current.HoraInicio, current.HoraFinal, current.TotalHoras, current.ExtrasDiurnas,
		current.ExtrasNocturnas, current.ExtrasDiurnasFestivas, current.ExtrasNocturnasFestivas,
		current.Seq, formatTime(current.UpdatedAt), ftEncoded, id,
	)
	if err != nil {
		return nil, false, fmt.Errorf("patching overtime entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("committing overtime entry patch: %w", err)
	}
	return &current, true, nil
}

func (s *store) listEntries(ctx context.Context, userID string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, fecha, solicitada_por, actividad, observaciones,
			hora_inicio, hora_final, total_horas, extras_diurnas, extras_nocturnas,
			extras_diurnas_festivas, extras_nocturnas_festivas, seq, updated_at, deleted_at
		FROM overtime_entries WHERE user_id = ? AND deleted_at IS NULL ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying overtime entries: %w", err)
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
		return nil, fmt.Errorf("iterating overtime entries: %w", err)
	}
	return entries, nil
}

// EntryChangesSince returns every overtime entry belonging to userID
// with seq > since (including soft-deleted ones, as tombstones),
// ordered by seq — merged by internal/sync into GET /sync/changes.
func EntryChangesSince(ctx context.Context, sqlDB *sql.DB, userID string, since int64) ([]Entry, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT id, user_id, fecha, solicitada_por, actividad, observaciones,
			hora_inicio, hora_final, total_horas, extras_diurnas, extras_nocturnas,
			extras_diurnas_festivas, extras_nocturnas_festivas, seq, updated_at, deleted_at
		FROM overtime_entries WHERE user_id = ? AND seq > ? ORDER BY seq ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("querying overtime entry changes: %w", err)
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
		return nil, fmt.Errorf("iterating overtime entry changes: %w", err)
	}
	return entries, nil
}

func scanEntry(row *sql.Rows) (Entry, error) {
	var e Entry
	var updatedAt string
	var deletedAt sql.NullString
	if err := row.Scan(
		&e.ID, &e.UserID, &e.Fecha, &e.SolicitadaPor, &e.Actividad, &e.Observaciones,
		&e.HoraInicio, &e.HoraFinal, &e.TotalHoras, &e.ExtrasDiurnas, &e.ExtrasNocturnas,
		&e.ExtrasDiurnasFestivas, &e.ExtrasNocturnasFestivas, &e.Seq, &updatedAt, &deletedAt,
	); err != nil {
		return Entry{}, fmt.Errorf("scanning overtime entry: %w", err)
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

// --- MonthMeta ---

// MonthMetaPatch carries the fields present in a PATCH request
// against a month's metadata — see specs/lww-por-campo.
type MonthMetaPatch struct {
	Colaborador db.Field[string]
	Cedula      db.Field[string]
}

// patchMonthMeta merges patch into the (userID, yearMonth) row field
// by field, each resolved independently against field_updated_at (LWW
// per field — see specs/lww-por-campo). Always returns the resulting
// row, even if every field in patch lost the LWW. changed reports
// whether anything actually applied.
func (s *store) patchMonthMeta(ctx context.Context, userID, yearMonth string, patch MonthMetaPatch, updatedAt time.Time) (m *MonthMeta, changed bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT user_id, year_month, colaborador, cedula, seq, updated_at, deleted_at, field_updated_at
		FROM overtime_month_meta WHERE user_id = ? AND year_month = ?
	`, userID, yearMonth)
	if err != nil {
		return nil, false, fmt.Errorf("loading overtime month meta for patch: %w", err)
	}
	current, fieldUpdatedAtRaw, err := scanMonthMetaWithFieldTimestamps(rows)
	_ = rows.Close()
	switch {
	case errors.Is(err, errNotFound):
		// overtime_month_meta has no POST (natural key, no client-
		// generated id — see specs/esquema-datos) — PATCH is its only
		// write endpoint, so it doubles as "create if missing", same as
		// the PUT it replaces. An empty field_updated_at means any
		// timestamp in patch wins unconditionally below.
		current = MonthMeta{UserID: userID, YearMonth: yearMonth}
		fieldUpdatedAtRaw = "{}"
	case err != nil:
		return nil, false, err
	}

	ft, err := db.ParseFieldTimestamps(fieldUpdatedAtRaw)
	if err != nil {
		return nil, false, err
	}

	if patch.Colaborador.Set && ft.Wins("colaborador", updatedAt) {
		current.Colaborador = patch.Colaborador.Value
		changed = true
	}
	if patch.Cedula.Set && ft.Wins("cedula", updatedAt) {
		current.Cedula = patch.Cedula.Value
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
		INSERT INTO overtime_month_meta (user_id, year_month, colaborador, cedula, seq, updated_at, field_updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, year_month) DO UPDATE SET
			colaborador = excluded.colaborador,
			cedula = excluded.cedula,
			seq = excluded.seq,
			updated_at = excluded.updated_at,
			deleted_at = NULL,
			field_updated_at = excluded.field_updated_at
	`, userID, yearMonth, current.Colaborador, current.Cedula, current.Seq, formatTime(current.UpdatedAt), ftEncoded)
	if err != nil {
		return nil, false, fmt.Errorf("patching overtime month meta: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("committing overtime month meta patch: %w", err)
	}
	return &current, true, nil
}

func (s *store) softDeleteMonthMeta(ctx context.Context, userID, yearMonth string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM overtime_month_meta WHERE user_id = ? AND year_month = ? AND deleted_at IS NULL`,
		userID, yearMonth,
	).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errNotFound
		}
		return 0, fmt.Errorf("checking overtime month meta: %w", err)
	}

	seq, err := db.NextSeq(ctx, tx, userID)
	if err != nil {
		return 0, err
	}
	now := formatTime(time.Now().UTC())

	if _, err := tx.ExecContext(ctx,
		`UPDATE overtime_month_meta SET deleted_at = ?, updated_at = ?, seq = ? WHERE user_id = ? AND year_month = ?`,
		now, now, seq, userID, yearMonth,
	); err != nil {
		return 0, fmt.Errorf("deleting overtime month meta: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing overtime month meta delete: %w", err)
	}
	return seq, nil
}

func (s *store) listMonthMeta(ctx context.Context, userID string) ([]MonthMeta, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, year_month, colaborador, cedula, seq, updated_at, deleted_at
		FROM overtime_month_meta WHERE user_id = ? AND deleted_at IS NULL ORDER BY year_month DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying overtime month meta: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []MonthMeta
	for rows.Next() {
		m, err := scanMonthMeta(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating overtime month meta: %w", err)
	}
	return records, nil
}

// MonthMetaChangesSince returns every month-meta record belonging to
// userID with seq > since (including soft-deleted ones, as
// tombstones), ordered by seq — merged by internal/sync.
func MonthMetaChangesSince(ctx context.Context, sqlDB *sql.DB, userID string, since int64) ([]MonthMeta, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT user_id, year_month, colaborador, cedula, seq, updated_at, deleted_at
		FROM overtime_month_meta WHERE user_id = ? AND seq > ? ORDER BY seq ASC
	`, userID, since)
	if err != nil {
		return nil, fmt.Errorf("querying overtime month meta changes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []MonthMeta
	for rows.Next() {
		m, err := scanMonthMeta(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating overtime month meta changes: %w", err)
	}
	return records, nil
}

func scanMonthMeta(row *sql.Rows) (MonthMeta, error) {
	var m MonthMeta
	var updatedAt string
	var deletedAt sql.NullString
	if err := row.Scan(&m.UserID, &m.YearMonth, &m.Colaborador, &m.Cedula, &m.Seq, &updatedAt, &deletedAt); err != nil {
		return MonthMeta{}, fmt.Errorf("scanning overtime month meta: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return MonthMeta{}, err
	}
	m.UpdatedAt = updated

	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return MonthMeta{}, err
		}
		m.DeletedAt = &d
	}

	return m, nil
}

// scanEntryWithFieldTimestamps reads the single row from a query
// selecting the same columns as scanEntry plus a trailing
// field_updated_at, returning errNotFound if there's no such row —
// used by patchEntry.
func scanEntryWithFieldTimestamps(rows *sql.Rows) (Entry, string, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Entry{}, "", fmt.Errorf("querying overtime entry: %w", err)
		}
		return Entry{}, "", errNotFound
	}

	var e Entry
	var updatedAt, fieldUpdatedAt string
	var deletedAt sql.NullString
	if err := rows.Scan(
		&e.ID, &e.UserID, &e.Fecha, &e.SolicitadaPor, &e.Actividad, &e.Observaciones,
		&e.HoraInicio, &e.HoraFinal, &e.TotalHoras, &e.ExtrasDiurnas, &e.ExtrasNocturnas,
		&e.ExtrasDiurnasFestivas, &e.ExtrasNocturnasFestivas, &e.Seq, &updatedAt, &deletedAt, &fieldUpdatedAt,
	); err != nil {
		return Entry{}, "", fmt.Errorf("scanning overtime entry: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return Entry{}, "", err
	}
	e.UpdatedAt = updated

	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return Entry{}, "", err
		}
		e.DeletedAt = &d
	}

	return e, fieldUpdatedAt, nil
}

// scanMonthMetaWithFieldTimestamps reads the single row from a query
// selecting the same columns as scanMonthMeta plus a trailing
// field_updated_at, returning errNotFound if there's no such row —
// used by patchMonthMeta.
func scanMonthMetaWithFieldTimestamps(rows *sql.Rows) (MonthMeta, string, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return MonthMeta{}, "", fmt.Errorf("querying overtime month meta: %w", err)
		}
		return MonthMeta{}, "", errNotFound
	}

	var m MonthMeta
	var updatedAt, fieldUpdatedAt string
	var deletedAt sql.NullString
	if err := rows.Scan(&m.UserID, &m.YearMonth, &m.Colaborador, &m.Cedula, &m.Seq, &updatedAt, &deletedAt, &fieldUpdatedAt); err != nil {
		return MonthMeta{}, "", fmt.Errorf("scanning overtime month meta: %w", err)
	}

	updated, err := parseTime(updatedAt)
	if err != nil {
		return MonthMeta{}, "", err
	}
	m.UpdatedAt = updated

	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return MonthMeta{}, "", err
		}
		m.DeletedAt = &d
	}

	return m, fieldUpdatedAt, nil
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
