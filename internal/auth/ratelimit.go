package auth

import (
	"sync"
	"time"
)

const (
	loginMaxAttempts = 5
	loginWindow      = time.Minute
)

// loginLimiter is a simple in-memory sliding-window limiter, keyed by
// caller (e.g. IP+email). In-memory is enough for the target deployment
// of this project: a single self-hosted instance, not a multi-replica
// cluster.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string][]time.Time)}
}

// Allow reports whether key is still under the attempt limit.
func (l *loginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key)) < loginMaxAttempts
}

// RecordFailure counts a failed attempt against key.
func (l *loginLimiter) RecordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[key] = append(l.prune(key), time.Now())
}

// Reset clears failed attempts for key, called after a successful login.
func (l *loginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

func (l *loginLimiter) prune(key string) []time.Time {
	cutoff := time.Now().Add(-loginWindow)
	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.attempts[key] = kept
	return kept
}
