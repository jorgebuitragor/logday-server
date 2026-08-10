package auth

import (
	"context"
	"testing"
)

func TestBootstrapWithoutEnvVarsIsNoLongerFatal(t *testing.T) {
	s := newTestStore(t)
	t.Setenv("ADMIN_EMAIL", "")
	t.Setenv("ADMIN_PASSWORD", "")

	if err := Bootstrap(context.Background(), s); err != nil {
		t.Fatalf("expected Bootstrap to succeed (deferring to /setup) without env vars, got %v", err)
	}

	count, err := s.countUsers(context.Background())
	if err != nil {
		t.Fatalf("countUsers: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no user to be created, got %d", count)
	}
}

func TestBootstrapWithEnvVarsCreatesAdmin(t *testing.T) {
	s := newTestStore(t)
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	t.Setenv("ADMIN_PASSWORD", "correct-horse-battery-staple")

	if err := Bootstrap(context.Background(), s); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	u, err := s.getUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("expected the env-var admin to exist: %v", err)
	}
	if !u.IsAdmin {
		t.Fatalf("expected the bootstrapped user to be an admin")
	}
}

func TestBootstrapIsNoOpWhenUsersAlreadyExist(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mustCreateUser(t, s, ctx, "existing@example.com", true)

	t.Setenv("ADMIN_EMAIL", "someone-else@example.com")
	t.Setenv("ADMIN_PASSWORD", "correct-horse-battery-staple")

	if err := Bootstrap(ctx, s); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	count, err := s.countUsers(ctx)
	if err != nil {
		t.Fatalf("countUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected Bootstrap to be a no-op once a user exists, got %d users", count)
	}
}
