// Package settings holds instance-wide operational config (the admin
// panel's "Configuración" section) — a singleton row read by several
// unrelated packages (internal/db's tombstone purge, internal/auth's
// login rate limiter and panel), so it lives on its own instead of
// inside any one of them, the same reasoning already used for
// internal/security and internal/crdt: no package here has business
// knowledge of users, devices, or synced domain data, it just reads and
// writes six plain columns.
package settings

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Settings is instance-wide config, stored as a single row
// (instance_settings.id = 1). Exported, unlike most domain-package
// structs in this codebase, because Get/Update are called directly by
// other packages — there's no store type to hide it behind.
type Settings struct {
	InstanceName                string
	TombstoneRetentionDays      int
	LoginRateLimitAttempts      int
	LoginRateLimitWindowSeconds int
	UpdatedAt                   time.Time
}

// TombstoneRetention is TombstoneRetentionDays as a time.Duration, for
// callers doing date arithmetic (internal/db.PurgeTombstones).
func (s Settings) TombstoneRetention() time.Duration {
	return time.Duration(s.TombstoneRetentionDays) * 24 * time.Hour
}

// LoginRateLimitWindow is LoginRateLimitWindowSeconds as a
// time.Duration, for internal/auth's login limiter.
func (s Settings) LoginRateLimitWindow() time.Duration {
	return time.Duration(s.LoginRateLimitWindowSeconds) * time.Second
}

// Get reads the current instance settings. The row always exists post-
// migration (00015_create_instance_settings.sql inserts it), so
// sql.ErrNoRows here would indicate a corrupted database, not a normal
// "not configured yet" state.
func Get(ctx context.Context, db *sql.DB) (*Settings, error) {
	var s Settings
	var updatedAt string
	err := db.QueryRowContext(ctx, `
		SELECT instance_name, tombstone_retention_days, login_rate_limit_attempts,
		       login_rate_limit_window_seconds, updated_at
		FROM instance_settings WHERE id = 1
	`).Scan(&s.InstanceName, &s.TombstoneRetentionDays, &s.LoginRateLimitAttempts,
		&s.LoginRateLimitWindowSeconds, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("reading instance settings: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parsing settings updated_at: %w", err)
	}
	s.UpdatedAt = t
	return &s, nil
}

// Update overwrites the instance settings row. Callers are responsible
// for validating field values before calling this — Update itself only
// enforces the DB-level CHECK on tombstone_retention_days etc. having
// sane types, not business-rule bounds (see internal/auth's panel
// handler for the actual min/max validation shown to the operator).
func Update(ctx context.Context, db *sql.DB, s Settings) error {
	_, err := db.ExecContext(ctx, `
		UPDATE instance_settings
		SET instance_name = ?, tombstone_retention_days = ?, login_rate_limit_attempts = ?,
		    login_rate_limit_window_seconds = ?, updated_at = ?
		WHERE id = 1
	`, s.InstanceName, s.TombstoneRetentionDays, s.LoginRateLimitAttempts,
		s.LoginRateLimitWindowSeconds, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("updating instance settings: %w", err)
	}
	return nil
}
