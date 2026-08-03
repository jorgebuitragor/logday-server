package auth

import "testing"

func TestLoginLimiterBlocksAfterMaxAttempts(t *testing.T) {
	l := newLoginLimiter()
	const key = "127.0.0.1:user@example.com"

	for i := 0; i < loginMaxAttempts; i++ {
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
	l := newLoginLimiter()
	const key = "127.0.0.1:user@example.com"

	for i := 0; i < loginMaxAttempts; i++ {
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
