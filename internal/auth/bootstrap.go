package auth

import (
	"context"
	"fmt"
	"log"
	"os"
)

// Bootstrap creates the first admin user from the ADMIN_EMAIL/
// ADMIN_PASSWORD environment variables if the instance has no users
// yet. It is a no-op on every later boot once at least one user exists.
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
		return fmt.Errorf("no users exist yet: set ADMIN_EMAIL and ADMIN_PASSWORD to bootstrap the first admin")
	}

	hash, err := hashPassword(password)
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
