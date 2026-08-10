package auth

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jorgebuitragor/logday-server/internal/security"
)

// Bootstrap creates the first admin user from the ADMIN_EMAIL/
// ADMIN_PASSWORD environment variables if the instance has no users
// yet. It is a no-op on every later boot once at least one user exists.
//
// If those env vars aren't set and the instance has no users, this is
// NOT an error: the server still boots, and GET /setup (see
// specs/panel-admin/) serves a one-time form to create the first admin
// from a browser instead.
func Bootstrap(ctx context.Context, s *store) error {
	count, err := s.countUsers(ctx)
	if err != nil {
		return fmt.Errorf("counting users: %w", err)
	}
	if count > 0 {
		return nil
	}

	email := os.Getenv("ADMIN_EMAIL")
	password := os.Getenv("ADMIN_PASSWORD")
	if email == "" || password == "" {
		log.Print("no admin user yet — visit /setup to create one, or set ADMIN_EMAIL/ADMIN_PASSWORD and restart")
		return nil
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hashing admin password: %w", err)
	}

	if _, err := s.createUser(ctx, email, hash, true); err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}

	//nolint:gosec // G706: email is operator-provided deploy config (ADMIN_EMAIL env var), not attacker input
	log.Printf("bootstrapped admin user %q", email)
	return nil
}
