package auth

import "testing"

// maxAttempts matches instance_settings' seeded default
// (login_rate_limit_attempts = 5, see migration 00015) — newTestStore's
// temp DB gets that row from the same migration set every other store
// test already relies on.
const maxAttemptsForTest = 5

func TestLoginLimiterBlocksAfterMaxAttempts(t *testing.T) {
	l := newLoginLimiter(newTestStore(t).db)
	const key = "127.0.0.1:user@example.com"

	for i := 0; i < maxAttemptsForTest; i++ {
		if !l.Allow(key) {
			t.Fatalf("expected attempt %d to be allowed", i+1)
		}
		l.RecordFailure(key)
	}

	if l.Allow(key) {
		t.Fatal("expected limiter to block after reaching the attempt limit")
	}
}

func TestLoginLimiterResetClearsAttempts(t *testing.T) {
	l := newLoginLimiter(newTestStore(t).db)
	const key = "127.0.0.1:user@example.com"

	for i := 0; i < maxAttemptsForTest; i++ {
		l.RecordFailure(key)
	}
	if l.Allow(key) {
		t.Fatal("expected limiter to be blocked before reset")
	}

	l.Reset(key)

	if !l.Allow(key) {
		t.Fatal("expected limiter to allow again after reset")
	}
}
