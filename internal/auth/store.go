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
	errLastAdmin      = errors.New("cannot remove the last active admin")
	errAlreadyInit    = errors.New("instance already has an admin")
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
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`).Scan(&count); err != nil {
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
		`SELECT id, email, password_hash, is_admin, created_at, deleted_at
		 FROM users WHERE email = ? AND deleted_at IS NULL`, email)
	return scanUser(row)
}

func (s *store) getUserByID(ctx context.Context, id string) (*user, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, is_admin, created_at, deleted_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*user, error) {
	var u user
	var createdAt string
	var deletedAt sql.NullString
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.IsAdmin, &createdAt, &deletedAt); err != nil {
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
	if deletedAt.Valid {
		d, err := parseTime(deletedAt.String)
		if err != nil {
			return nil, err
		}
		u.DeletedAt = &d
	}
	return &u, nil
}

func (s *store) createDevice(ctx context.Context, userID, deviceName, refreshTokenHash string, expiresAt time.Time, ip, userAgent string) (*device, error) {
	now := time.Now().UTC()
	d := &device{
		ID:                    uuid.NewString(),
		UserID:                userID,
		DeviceName:            deviceName,
		RefreshTokenHash:      refreshTokenHash,
		RefreshTokenExpiresAt: expiresAt,
		CreatedAt:             now,
		LastUsedAt:            now,
		LastIP:                ip,
		UserAgent:             userAgent,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO devices (id, user_id, device_name, refresh_token_hash, refresh_token_expires_at, created_at, last_used_at, last_ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.UserID, d.DeviceName, d.RefreshTokenHash,
		formatTime(d.RefreshTokenExpiresAt), formatTime(d.CreatedAt), formatTime(d.LastUsedAt), d.LastIP, d.UserAgent,
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
func (s *store) rotateRefreshToken(ctx context.Context, deviceID, oldHash, newHash string, newExpiresAt time.Time, refreshTTL time.Duration, ip, userAgent string) error {
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
		`UPDATE devices SET refresh_token_hash = ?, refresh_token_expires_at = ?, last_used_at = ?, last_ip = ?, user_agent = ? WHERE id = ?`,
		newHash, formatTime(newExpiresAt), formatTime(now), ip, userAgent, deviceID,
	); err != nil {
		return fmt.Errorf("updating device: %w", err)
	}

	cutoff := formatTime(now.Add(-refreshTTL - 24*time.Hour))
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

// countDevices backs the configurable max-devices-per-user guard on
// login (Settings.MaxDevicesPerUser).
func (s *store) countDevices(ctx context.Context, userID string) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE user_id = ?`, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting devices: %w", err)
	}
	return count, nil
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

// listUsers returns every user in the instance, active and soft-deleted
// — the admin panel is the only caller that needs to see both, so it
// doesn't scope by deleted_at like the rest of the store does.
func (s *store) listUsers(ctx context.Context) ([]user, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, password_hash, is_admin, created_at, deleted_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("querying users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []user
	for rows.Next() {
		var u user
		var createdAt string
		var deletedAt sql.NullString
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.IsAdmin, &createdAt, &deletedAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		t, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		u.CreatedAt = t
		if deletedAt.Valid {
			d, err := parseTime(deletedAt.String)
			if err != nil {
				return nil, err
			}
			u.DeletedAt = &d
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating users: %w", err)
	}
	return users, nil
}

// updateUserAdmin promotes or demotes id. Demoting the sole active admin
// is rejected (errLastAdmin) rather than locking the instance out of its
// own panel — withLastAdminGuard's check is a no-op for promotions
// (the target isn't currently an admin, so the guard never triggers).
func (s *store) updateUserAdmin(ctx context.Context, id string, isAdmin bool) error {
	return s.withLastAdminGuard(ctx, id, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE users SET is_admin = ? WHERE id = ?`, isAdmin, id)
		if err != nil {
			return fmt.Errorf("updating user admin flag: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		if affected == 0 {
			return errNotFound
		}
		return nil
	})
}

// softDeleteUser deactivates id and immediately revokes all of its
// devices — deleted_at alone wouldn't stop an already-issued refresh
// token from working, since the ON DELETE CASCADE on devices only fires
// on an actual DELETE of the user row, not this UPDATE.
func (s *store) softDeleteUser(ctx context.Context, id string) error {
	return s.withLastAdminGuard(ctx, id, func(tx *sql.Tx) error {
		now := formatTime(time.Now().UTC())
		res, err := tx.ExecContext(ctx,
			`UPDATE users SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, now, id)
		if err != nil {
			return fmt.Errorf("soft-deleting user: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		if affected == 0 {
			return errNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM devices WHERE user_id = ?`, id); err != nil {
			return fmt.Errorf("revoking devices: %w", err)
		}
		return nil
	})
}

// withLastAdminGuard runs fn inside a transaction after confirming id
// isn't the instance's sole remaining active admin (a no-op check when
// id isn't currently an admin at all — safe to use unconditionally for
// both updateUserAdmin's promote and demote paths, and for
// softDeleteUser). updateUserPassword doesn't need this guard: a
// password reset never changes admin status.
func (s *store) withLastAdminGuard(ctx context.Context, id string, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var isAdmin bool
	err = tx.QueryRowContext(ctx, `SELECT is_admin FROM users WHERE id = ? AND deleted_at IS NULL`, id).Scan(&isAdmin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNotFound
		}
		return fmt.Errorf("checking user: %w", err)
	}
	if isAdmin {
		var admins int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND is_admin = 1`,
		).Scan(&admins); err != nil {
			return fmt.Errorf("counting admins: %w", err)
		}
		if admins <= 1 {
			return errLastAdmin
		}
	}

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	return nil
}

// restoreUser reactivates a soft-deleted user. Without this, soft-delete
// would have no practical advantage over a hard delete.
func (s *store) restoreUser(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET deleted_at = NULL WHERE id = ? AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("restoring user: %w", err)
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

// updateUserPassword resets id's password hash (admin-assisted, not
// self-service — see specs/panel-admin/). Also revokes all of the
// user's devices: a password reset is exactly the kind of event where
// forcing re-login everywhere is the correct default.
func (s *store) updateUserPassword(ctx context.Context, id, passwordHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE id = ? AND deleted_at IS NULL`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if affected == 0 {
		return errNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM devices WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("revoking devices: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing password reset: %w", err)
	}
	return nil
}

// listAllDevices returns every device in the instance, joined with its
// owning user's email — unlike listDevices, deliberately not scoped to
// one user, for the admin panel's cross-user device view.
func (s *store) listAllDevices(ctx context.Context) ([]deviceWithOwner, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.user_id, d.device_name, d.refresh_token_hash, d.refresh_token_expires_at,
		       d.created_at, d.last_used_at, d.last_ip, d.user_agent, u.email
		FROM devices d
		JOIN users u ON u.id = d.user_id
		ORDER BY d.last_used_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying devices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []deviceWithOwner
	for rows.Next() {
		var dw deviceWithOwner
		var expiresAt, createdAt, lastUsedAt string
		if err := rows.Scan(&dw.ID, &dw.UserID, &dw.DeviceName, &dw.RefreshTokenHash, &expiresAt,
			&createdAt, &lastUsedAt, &dw.LastIP, &dw.UserAgent, &dw.OwnerEmail); err != nil {
			return nil, fmt.Errorf("scanning device: %w", err)
		}
		if _, err := fillDeviceTimes(&dw.device, expiresAt, createdAt, lastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, dw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating devices: %w", err)
	}
	return out, nil
}

// createFirstAdmin atomically checks that the instance has no active
// users yet and creates email/hash as its first admin, all inside one
// transaction — a plain "check then insert" would be a TOCTOU race
// between two concurrent /setup submissions.
func (s *store) createFirstAdmin(ctx context.Context, email, passwordHash string) (*user, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`).Scan(&count); err != nil {
		return nil, fmt.Errorf("counting users: %w", err)
	}
	if count > 0 {
		return nil, errAlreadyInit
	}

	u := &user{
		ID:           uuid.NewString(),
		Email:        email,
		PasswordHash: passwordHash,
		IsAdmin:      true,
		CreatedAt:    time.Now().UTC(),
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, is_admin, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.PasswordHash, u.IsAdmin, formatTime(u.CreatedAt),
	); err != nil {
		if isUniqueConstraintErr(err) {
			return nil, errDuplicateEmail
		}
		return nil, fmt.Errorf("inserting first admin: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing first admin creation: %w", err)
	}
	return u, nil
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
