package auth

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/db"
)

func newTestStore(t *testing.T) *store {
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
	return NewStore(database)
}

func mustCreateUser(t *testing.T, s *store, ctx context.Context, email string, isAdmin bool) *user {
	t.Helper()
	u, err := s.createUser(ctx, email, "hash", isAdmin)
	if err != nil {
		t.Fatalf("createUser(%q): %v", email, err)
	}
	return u
}

func TestListUsersReturnsActiveAndDeleted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	admin := mustCreateUser(t, s, ctx, "admin@example.com", true)
	member := mustCreateUser(t, s, ctx, "member@example.com", false)
	if err := s.softDeleteUser(ctx, member.ID); err != nil {
		t.Fatalf("softDeleteUser: %v", err)
	}

	users, err := s.listUsers(ctx)
	if err != nil {
		t.Fatalf("listUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users (active+deleted), got %d: %+v", len(users), users)
	}

	byID := map[string]user{}
	for _, u := range users {
		byID[u.ID] = u
	}
	if byID[admin.ID].DeletedAt != nil {
		t.Fatalf("expected admin to still be active, got deleted_at=%v", byID[admin.ID].DeletedAt)
	}
	if byID[member.ID].DeletedAt == nil {
		t.Fatalf("expected member to be soft-deleted")
	}
}

func TestUpdateUserAdminPromoteAndDemote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	admin := mustCreateUser(t, s, ctx, "admin@example.com", true)
	member := mustCreateUser(t, s, ctx, "member@example.com", false)

	if err := s.updateUserAdmin(ctx, member.ID, true); err != nil {
		t.Fatalf("promote: %v", err)
	}
	got, err := s.getUserByID(ctx, member.ID)
	if err != nil {
		t.Fatalf("getUserByID: %v", err)
	}
	if !got.IsAdmin {
		t.Fatalf("expected member to be promoted")
	}

	if err := s.updateUserAdmin(ctx, member.ID, false); err != nil {
		t.Fatalf("demote (two admins left): %v", err)
	}

	// Demoting the sole remaining admin must be rejected, and must not
	// change the row.
	if err := s.updateUserAdmin(ctx, admin.ID, false); !errors.Is(err, errLastAdmin) {
		t.Fatalf("expected errLastAdmin demoting the sole admin, got %v", err)
	}
	got, err = s.getUserByID(ctx, admin.ID)
	if err != nil {
		t.Fatalf("getUserByID: %v", err)
	}
	if !got.IsAdmin {
		t.Fatalf("expected the rejected demote to leave admin.IsAdmin unchanged (still true)")
	}
}

func TestSoftDeleteUserRevokesDevicesAndBlocksLogin(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	admin := mustCreateUser(t, s, ctx, "admin@example.com", true)
	member := mustCreateUser(t, s, ctx, "member@example.com", false)
	if _, err := s.createDevice(ctx, member.ID, "laptop", "somehash", time.Now().Add(time.Hour), "127.0.0.1", "test-agent"); err != nil {
		t.Fatalf("createDevice: %v", err)
	}

	if err := s.softDeleteUser(ctx, member.ID); err != nil {
		t.Fatalf("softDeleteUser: %v", err)
	}

	devices, err := s.listDevices(ctx, member.ID)
	if err != nil {
		t.Fatalf("listDevices: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected member's devices to be revoked, got %+v", devices)
	}

	if _, err := s.getUserByEmail(ctx, "member@example.com"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected a soft-deleted user to be unable to log in (errNotFound), got %v", err)
	}

	// Deleting the sole admin must be rejected, same guard as demote.
	if err := s.softDeleteUser(ctx, admin.ID); !errors.Is(err, errLastAdmin) {
		t.Fatalf("expected errLastAdmin deleting the sole admin, got %v", err)
	}
}

func TestRestoreUserReversesSoftDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mustCreateUser(t, s, ctx, "admin@example.com", true)
	member := mustCreateUser(t, s, ctx, "member@example.com", false)
	if err := s.softDeleteUser(ctx, member.ID); err != nil {
		t.Fatalf("softDeleteUser: %v", err)
	}

	if err := s.restoreUser(ctx, member.ID); err != nil {
		t.Fatalf("restoreUser: %v", err)
	}

	got, err := s.getUserByEmail(ctx, "member@example.com")
	if err != nil {
		t.Fatalf("expected restored user to be able to log in again: %v", err)
	}
	if got.DeletedAt != nil {
		t.Fatalf("expected DeletedAt to be cleared, got %v", got.DeletedAt)
	}

	if err := s.restoreUser(ctx, "does-not-exist"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound restoring a nonexistent user, got %v", err)
	}
}

func TestUpdateUserPasswordRevokesDevices(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	member := mustCreateUser(t, s, ctx, "member@example.com", false)
	if _, err := s.createDevice(ctx, member.ID, "laptop", "somehash", time.Now().Add(time.Hour), "127.0.0.1", "test-agent"); err != nil {
		t.Fatalf("createDevice: %v", err)
	}

	if err := s.updateUserPassword(ctx, member.ID, "new-hash"); err != nil {
		t.Fatalf("updateUserPassword: %v", err)
	}

	got, err := s.getUserByID(ctx, member.ID)
	if err != nil {
		t.Fatalf("getUserByID: %v", err)
	}
	if got.PasswordHash != "new-hash" {
		t.Fatalf("expected password hash to be updated, got %q", got.PasswordHash)
	}

	devices, err := s.listDevices(ctx, member.ID)
	if err != nil {
		t.Fatalf("listDevices: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected devices to be revoked after a password reset, got %+v", devices)
	}
}

func TestListAllDevicesIsUnscopedAndIncludesOwnerEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a := mustCreateUser(t, s, ctx, "a@example.com", true)
	b := mustCreateUser(t, s, ctx, "b@example.com", false)
	if _, err := s.createDevice(ctx, a.ID, "a-laptop", "hash-a", time.Now().Add(time.Hour), "10.0.0.1", "device-a-agent"); err != nil {
		t.Fatalf("createDevice a: %v", err)
	}
	if _, err := s.createDevice(ctx, b.ID, "b-phone", "hash-b", time.Now().Add(time.Hour), "10.0.0.2", "device-b-agent"); err != nil {
		t.Fatalf("createDevice b: %v", err)
	}

	devices, err := s.listAllDevices(ctx)
	if err != nil {
		t.Fatalf("listAllDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices across both users, got %d", len(devices))
	}
	byOwner := map[string]string{}
	for _, d := range devices {
		byOwner[d.DeviceName] = d.OwnerEmail
	}
	if byOwner["a-laptop"] != "a@example.com" || byOwner["b-phone"] != "b@example.com" {
		t.Fatalf("unexpected owner emails: %+v", byOwner)
	}
}

func TestCreateFirstAdminOnlySucceedsOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u, err := s.createFirstAdmin(ctx, "admin@example.com", "hash")
	if err != nil {
		t.Fatalf("first createFirstAdmin: %v", err)
	}
	if !u.IsAdmin {
		t.Fatalf("expected the first admin to have IsAdmin=true")
	}

	if _, err := s.createFirstAdmin(ctx, "someone-else@example.com", "hash2"); !errors.Is(err, errAlreadyInit) {
		t.Fatalf("expected errAlreadyInit on second call, got %v", err)
	}

	users, err := s.listUsers(ctx)
	if err != nil {
		t.Fatalf("listUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected exactly one user after the rejected second call, got %d", len(users))
	}
}

func TestCreateFirstAdminConcurrentCallsOnlyOneWins(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	const attempts = 10
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			_, err := s.createFirstAdmin(ctx, "admin@example.com", "hash")
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, errAlreadyInit) && !errors.Is(err, errDuplicateEmail) {
			t.Fatalf("unexpected error from concurrent createFirstAdmin: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 concurrent createFirstAdmin call to succeed, got %d", successes)
	}

	count, err := s.countUsers(ctx)
	if err != nil {
		t.Fatalf("countUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 user in the database, got %d", count)
	}
}
