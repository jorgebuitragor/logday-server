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
	"strings"
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
	AllowedEmailDomains         string // CSV, minúsculas; "" = cualquier dominio
	MinPasswordLength           int
	AccessTokenTTLMinutes       int
	RefreshTokenTTLDays         int
	PanelSessionTTLHours        int
	MaxDevicesPerUser           int // 0 = sin límite
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

// AccessTokenTTL is AccessTokenTTLMinutes as a time.Duration.
func (s Settings) AccessTokenTTL() time.Duration {
	return time.Duration(s.AccessTokenTTLMinutes) * time.Minute
}

// RefreshTokenTTL is RefreshTokenTTLDays as a time.Duration.
func (s Settings) RefreshTokenTTL() time.Duration {
	return time.Duration(s.RefreshTokenTTLDays) * 24 * time.Hour
}

// PanelSessionTTL is PanelSessionTTLHours as a time.Duration.
func (s Settings) PanelSessionTTL() time.Duration {
	return time.Duration(s.PanelSessionTTLHours) * time.Hour
}

// EmailDomainAllowed reports whether email's domain passes the instance's
// allowlist. An empty AllowedEmailDomains means no restriction — every
// domain is allowed, matching today's behavior for instances that never
// touch this setting. This is an operational guardrail (catching a
// fat-fingered domain when an admin invites someone), not an access
// control: every user-creation path already requires an authenticated
// admin, there's no public self-signup this could gate.
func (s Settings) EmailDomainAllowed(email string) bool {
	allowed := strings.TrimSpace(s.AllowedEmailDomains)
	if allowed == "" {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range strings.Split(allowed, ",") {
		if strings.ToLower(strings.TrimSpace(d)) == domain {
			return true
		}
	}
	return false
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
		       login_rate_limit_window_seconds, allowed_email_domains, min_password_length,
		       access_token_ttl_minutes, refresh_token_ttl_days, panel_session_ttl_hours,
		       max_devices_per_user, updated_at
		FROM instance_settings WHERE id = 1
	`).Scan(&s.InstanceName, &s.TombstoneRetentionDays, &s.LoginRateLimitAttempts,
		&s.LoginRateLimitWindowSeconds, &s.AllowedEmailDomains, &s.MinPasswordLength,
		&s.AccessTokenTTLMinutes, &s.RefreshTokenTTLDays, &s.PanelSessionTTLHours,
		&s.MaxDevicesPerUser, &updatedAt)
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
		    login_rate_limit_window_seconds = ?, allowed_email_domains = ?, min_password_length = ?,
		    access_token_ttl_minutes = ?, refresh_token_ttl_days = ?, panel_session_ttl_hours = ?,
		    max_devices_per_user = ?, updated_at = ?
		WHERE id = 1
	`, s.InstanceName, s.TombstoneRetentionDays, s.LoginRateLimitAttempts,
		s.LoginRateLimitWindowSeconds, s.AllowedEmailDomains, s.MinPasswordLength,
		s.AccessTokenTTLMinutes, s.RefreshTokenTTLDays, s.PanelSessionTTLHours,
		s.MaxDevicesPerUser, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("updating instance settings: %w", err)
	}
	return nil
}
