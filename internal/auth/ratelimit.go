package auth

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/settings"
)

// fallbackMaxAttempts/fallbackWindow are used only if reading current
// settings fails (e.g. a transient DB error) — fail toward the
// documented default rather than either wide open or fully locked.
const (
	fallbackMaxAttempts = 5
	fallbackWindow      = time.Minute
)

// loginLimiter is a simple in-memory sliding-window limiter, keyed by
// caller (e.g. IP+email). In-memory is enough for the target deployment
// of this project: a single self-hosted instance, not a multi-replica
// cluster. The limit/window themselves are read live from
// instance_settings on every check (a single indexed PK read — login
// attempts are nowhere near a hot path), so a change made in the admin
// panel's Configuración section takes effect immediately, no restart.
type loginLimiter struct {
	db *sql.DB

	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newLoginLimiter(db *sql.DB) *loginLimiter {
	return &loginLimiter{db: db, attempts: make(map[string][]time.Time)}
}

func (l *loginLimiter) limits() (maxAttempts int, window time.Duration) {
	cfg, err := settings.Get(context.Background(), l.db)
	if err != nil {
		log.Printf("reading login rate limit settings, using fallback: %v", err)
		return fallbackMaxAttempts, fallbackWindow
	}
	return cfg.LoginRateLimitAttempts, cfg.LoginRateLimitWindow()
}

// Allow reports whether key is still under the attempt limit.
func (l *loginLimiter) Allow(key string) bool {
	maxAttempts, window := l.limits()
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key, window)) < maxAttempts
}

// RecordFailure counts a failed attempt against key.
func (l *loginLimiter) RecordFailure(key string) {
	_, window := l.limits()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[key] = append(l.prune(key, window), time.Now())
}

// Reset clears failed attempts for key, called after a successful login.
func (l *loginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

func (l *loginLimiter) prune(key string, window time.Duration) []time.Time {
	cutoff := time.Now().Add(-window)
	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.attempts[key] = kept
	return kept
}
