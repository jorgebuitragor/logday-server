// External test package (settings_test, not settings) on purpose:
// internal/db imports internal/settings (for PurgeTombstones' retention
// setting), so a same-package test here that also imports internal/db
// to build a test database would create an import cycle. The external
// test package sidesteps that — it's a distinct package from
// settings' own perspective.
package settings_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/db"
	"github.com/jorgebuitragor/logday-server/internal/settings"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return database
}

func TestGetReturnsMigrationSeededDefaults(t *testing.T) {
	database := newTestDB(t)

	s, err := settings.Get(context.Background(), database)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if s.InstanceName != "Logday Server" {
		t.Fatalf("expected default instance name, got %q", s.InstanceName)
	}
	if s.TombstoneRetentionDays != 90 {
		t.Fatalf("expected default retention of 90 days, got %d", s.TombstoneRetentionDays)
	}
	if s.LoginRateLimitAttempts != 5 {
		t.Fatalf("expected default rate limit of 5 attempts, got %d", s.LoginRateLimitAttempts)
	}
	if s.LoginRateLimitWindowSeconds != 60 {
		t.Fatalf("expected default rate limit window of 60s, got %d", s.LoginRateLimitWindowSeconds)
	}
	if s.TombstoneRetention() != 90*24*time.Hour {
		t.Fatalf("unexpected TombstoneRetention(): %v", s.TombstoneRetention())
	}
	if s.LoginRateLimitWindow() != 60*time.Second {
		t.Fatalf("unexpected LoginRateLimitWindow(): %v", s.LoginRateLimitWindow())
	}
}

func TestUpdateRoundTrips(t *testing.T) {
	database := newTestDB(t)
	ctx := context.Background()

	before, err := settings.Get(ctx, database)
	if err != nil {
		t.Fatalf("Get before update: %v", err)
	}

	want := settings.Settings{
		InstanceName:                "Equipo de Producto",
		TombstoneRetentionDays:      30,
		LoginRateLimitAttempts:      10,
		LoginRateLimitWindowSeconds: 120,
	}
	if err := settings.Update(ctx, database, want); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := settings.Get(ctx, database)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.InstanceName != want.InstanceName ||
		got.TombstoneRetentionDays != want.TombstoneRetentionDays ||
		got.LoginRateLimitAttempts != want.LoginRateLimitAttempts ||
		got.LoginRateLimitWindowSeconds != want.LoginRateLimitWindowSeconds {
		t.Fatalf("Update did not persist: got %+v, want %+v", got, want)
	}
	if !got.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("expected UpdatedAt to advance: before=%v after=%v", before.UpdatedAt, got.UpdatedAt)
	}
}
