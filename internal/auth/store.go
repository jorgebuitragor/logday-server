package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
)

var (
	errNotFound       = errors.New("not found")
	errDuplicateEmail = errors.New("email already exists")
)

type store struct {
	db *sql.DB
}

// NewStore wires up the SQL-backed store used by Bootstrap and by the
// HTTP handlers returned from NewHandler.
func NewStore(db *sql.DB) *store {
	return &store{db: db}
}

func (s *store) countUsers(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}

func (s *store) createUser(ctx context.Context, email, passwordHash string, isAdmin bool) (*user, error) {
	u := &user{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: passwordHash,
		IsAdmin:      isAdmin,
		CreatedAt:    time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, is_admin, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.PasswordHash, u.IsAdmin, formatTime(u.CreatedAt),
	)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return nil, errDuplicateEmail
		}
		return nil, fmt.Errorf("inserting user: %w", err)
	}
	return u, nil
}

func (s *store) getUserByEmail(ctx context.Context, email string) (*user, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, is_admin, created_at FROM users WHERE email = ?`, email)
	return scanUser(row)
}

func (s *store) getUserByID(ctx context.Context, id string) (*user, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, is_admin, created_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*user, error) {
	var u user
	var createdAt string
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.IsAdmin, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errNotFound
		}
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	t, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt = t
	return &u, nil
}

func (s *store) createDevice(ctx context.Context, userID, deviceName, refreshTokenHash string, expiresAt time.Time) (*device, error) {
	now := time.Now().UTC()
	d := &device{
		ID:                    uuid.NewString(),
		UserID:                userID,
		DeviceName:            deviceName,
		RefreshTokenHash:      refreshTokenHash,
		RefreshTokenExpiresAt: expiresAt,
		CreatedAt:             now,
		LastUsedAt:            now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO devices (id, user_id, device_name, refresh_token_hash, refresh_token_expires_at, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.UserID, d.DeviceName, d.RefreshTokenHash,
		formatTime(d.RefreshTokenExpiresAt), formatTime(d.CreatedAt), formatTime(d.LastUsedAt),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting device: %w", err)
	}
	return d, nil
}

func (s *store) getDeviceByRefreshTokenHash(ctx context.Context, hash string) (*device, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, device_name, refresh_token_hash, refresh_token_expires_at, created_at, last_used_at
		 FROM devices WHERE refresh_token_hash = ?`, hash)
	return scanDevice(row)
}

func scanDevice(row *sql.Row) (*device, error) {
	var d device
	var expiresAt, createdAt, lastUsedAt string
	if err := row.Scan(&d.ID, &d.UserID, &d.DeviceName, &d.RefreshTokenHash, &expiresAt, &createdAt, &lastUsedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errNotFound
		}
		return nil, fmt.Errorf("scanning device: %w", err)
	}
	return fillDeviceTimes(&d, expiresAt, createdAt, lastUsedAt)
}

func fillDeviceTimes(d *device, expiresAt, createdAt, lastUsedAt string) (*device, error) {
	var err error
	if d.RefreshTokenExpiresAt, err = parseTime(expiresAt); err != nil {
		return nil, err
	}
	if d.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if d.LastUsedAt, err = parseTime(lastUsedAt); err != nil {
		return nil, err
	}
	return d, nil
}

// rotateRefreshToken replaces a device's refresh token, recording the
// old hash as used so a later replay attempt can be detected as token
// theft (see wasRefreshTokenUsed) instead of failing like any other
// invalid token.
func (s *store) rotateRefreshToken(ctx context.Context, deviceID, oldHash, newHash string, newExpiresAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO used_refresh_tokens (token_hash, device_id, used_at) VALUES (?, ?, ?)`,
		oldHash, deviceID, formatTime(now),
	); err != nil {
		return fmt.Errorf("recording used refresh token: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE devices SET refresh_token_hash = ?, refresh_token_expires_at = ?, last_used_at = ? WHERE id = ?`,
		newHash, formatTime(newExpiresAt), formatTime(now), deviceID,
	); err != nil {
		return fmt.Errorf("updating device: %w", err)
	}

	cutoff := formatTime(now.Add(-refreshTokenTTL - 24*time.Hour))
	if _, err := tx.ExecContext(ctx, `DELETE FROM used_refresh_tokens WHERE used_at < ?`, cutoff); err != nil {
		return fmt.Errorf("pruning used refresh tokens: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing rotation: %w", err)
	}
	return nil
}

// wasRefreshTokenUsed reports whether hash belongs to a refresh token
// that was already rotated away.
func (s *store) wasRefreshTokenUsed(ctx context.Context, hash string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM used_refresh_tokens WHERE token_hash = ?`, hash).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking used refresh token: %w", err)
	}
	return true, nil
}

// revokeDeviceByUsedTokenHash deletes the device associated with an
// already-rotated refresh token hash — used when a reuse attempt
// signals possible token theft.
func (s *store) revokeDeviceByUsedTokenHash(ctx context.Context, hash string) error {
	var deviceID string
	err := s.db.QueryRowContext(ctx, `SELECT device_id FROM used_refresh_tokens WHERE token_hash = ?`, hash).Scan(&deviceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNotFound
		}
		return fmt.Errorf("looking up used refresh token: %w", err)
	}
	return s.deleteDevice(ctx, deviceID)
}

func (s *store) deleteDevice(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM devices WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting device: %w", err)
	}
	return nil
}

// revokeDeviceForUser deletes a device, scoped to userID so one user
// can't revoke another user's device.
func (s *store) revokeDeviceForUser(ctx context.Context, id, userID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM devices WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("deleting device: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if affected == 0 {
		return errNotFound
	}
	return nil
}

func (s *store) listDevices(ctx context.Context, userID string) ([]device, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, device_name, refresh_token_hash, refresh_token_expires_at, created_at, last_used_at
		 FROM devices WHERE user_id = ? ORDER BY last_used_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying devices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var devices []device
	for rows.Next() {
		var d device
		var expiresAt, createdAt, lastUsedAt string
		if err := rows.Scan(&d.ID, &d.UserID, &d.DeviceName, &d.RefreshTokenHash, &expiresAt, &createdAt, &lastUsedAt); err != nil {
			return nil, fmt.Errorf("scanning device: %w", err)
		}
		if _, err := fillDeviceTimes(&d, expiresAt, createdAt, lastUsedAt); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating devices: %w", err)
	}
	return devices, nil
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

func isUniqueConstraintErr(err error) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code == sqlite3.ErrConstraint
	}
	return false
}
