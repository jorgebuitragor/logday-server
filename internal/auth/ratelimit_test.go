package auth

import (
	"fmt"
	"testing"
	"time"
)

// maxAttempts matches instance_settings' seeded default
// (login_rate_limit_attempts = 5, window 60 s, see migration 00015) —
// newTestStore's temp DB gets that row from the same migration set
// every other store test already relies on.
const (
	maxAttemptsForTest = 5
	windowForTest      = time.Minute
)

// newClockedLimiter returns a limiter whose clock only moves when the
// test calls advance.
func newClockedLimiter(t *testing.T) (*loginLimiter, func(time.Duration)) {
	t.Helper()
	l := newLoginLimiter(newTestStore(t).db)
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	return l, func(d time.Duration) { now = now.Add(d) }
}

func failN(l *loginLimiter, ip, email string, n int) {
	for i := 0; i < n; i++ {
		l.RecordFailure(ip, email)
	}
}

func TestLoginLimiterBlocksIPAndEmailAfterMaxAttempts(t *testing.T) {
	l, advance := newClockedLimiter(t)

	for i := 0; i < maxAttemptsForTest; i++ {
		if wait := l.Check("10.0.0.1", "user@example.com"); wait != 0 {
			t.Fatalf("attempt %d: want allowed, got wait %v", i+1, wait)
		}
		l.RecordFailure("10.0.0.1", "user@example.com")
		advance(time.Second)
	}

	// The oldest failure was 5 s ago: it leaves the window in 55 s.
	if wait := l.Check("10.0.0.1", "User@Example.com "); wait != windowForTest-5*time.Second {
		t.Fatalf("want 55s wait (email compared case-insensitively), got %v", wait)
	}
	// Another address is not held back by this pair, and the account
	// is still within its free failures.
	if wait := l.Check("10.0.0.2", "user@example.com"); wait != 0 {
		t.Fatalf("want another IP allowed, got %v", wait)
	}

	advance(windowForTest)
	if wait := l.Check("10.0.0.1", "user@example.com"); wait != 0 {
		t.Fatalf("want allowed once the window passes, got %v", wait)
	}
}

func TestLoginLimiterResetClearsPairAndAccount(t *testing.T) {
	l, _ := newClockedLimiter(t)
	failN(l, "10.0.0.1", "user@example.com", maxAttemptsForTest+1)
	if l.Check("10.0.0.1", "user@example.com") == 0 {
		t.Fatal("want blocked before reset")
	}

	l.Reset("10.0.0.1", "user@example.com")

	if wait := l.Check("10.0.0.1", "user@example.com"); wait != 0 {
		t.Fatalf("want allowed after reset, got %v", wait)
	}
}

func TestLoginLimiterBlocksIPAcrossEmails(t *testing.T) {
	l, _ := newClockedLimiter(t)
	limit := ipAttemptsFactor * maxAttemptsForTest

	// One failure per email: no pair or account reaches its limit.
	for i := 0; i < limit; i++ {
		email := fmt.Sprintf("user%d@example.com", i)
		if wait := l.Check("10.0.0.1", email); wait != 0 {
			t.Fatalf("attempt %d: want allowed, got %v", i+1, wait)
		}
		l.RecordFailure("10.0.0.1", email)
	}

	if l.Check("10.0.0.1", "fresh@example.com") == 0 {
		t.Fatal("want the IP blocked after spraying many emails")
	}
	if wait := l.Check("10.0.0.2", "fresh@example.com"); wait != 0 {
		t.Fatalf("another IP should be allowed, got %v", wait)
	}
}

func TestLoginLimiterBacksOffAccountAcrossIPs(t *testing.T) {
	l, advance := newClockedLimiter(t)
	ip := func(i int) string { return fmt.Sprintf("10.0.1.%d", i) }

	// The free failures, each from a different address: no wait yet.
	for i := 0; i < maxAttemptsForTest; i++ {
		l.RecordFailure(ip(i), "victim@example.com")
	}
	if wait := l.Check("10.9.9.9", "victim@example.com"); wait != 0 {
		t.Fatalf("want no account wait within the free failures, got %v", wait)
	}

	// Each failure past them doubles the wait, from any address.
	want := accountBaseDelay
	for i := maxAttemptsForTest; i < maxAttemptsForTest+4; i++ {
		l.RecordFailure(ip(i), "victim@example.com")
		if wait := l.Check("10.9.9.9", "victim@example.com"); wait != want {
			t.Fatalf("failure %d: want wait %v, got %v", i+1, want, wait)
		}
		advance(want)
		want *= 2
	}

	// Other accounts are unaffected.
	if wait := l.Check("10.9.9.9", "other@example.com"); wait != 0 {
		t.Fatalf("want other accounts allowed, got %v", wait)
	}
}

func TestBackoffDelayIsCapped(t *testing.T) {
	cases := map[int]time.Duration{
		1:   time.Second,
		2:   2 * time.Second,
		9:   256 * time.Second,
		10:  accountMaxDelay,
		500: accountMaxDelay,
	}
	for n, want := range cases {
		if got := backoffDelay(n); got != want {
			t.Errorf("backoffDelay(%d) = %v, want %v", n, got, want)
		}
	}
}

func TestLoginLimiterForgetsAccountAfterQuietPeriod(t *testing.T) {
	l, advance := newClockedLimiter(t)
	failN(l, "10.0.0.1", "user@example.com", maxAttemptsForTest)

	advance(accountForget + time.Second)
	l.RecordFailure("10.0.0.2", "user@example.com")

	// Counting restarted at one, so this failure is free again.
	if wait := l.Check("10.0.0.3", "user@example.com"); wait != 0 {
		t.Fatalf("want the count restarted after a quiet hour, got %v", wait)
	}
}

func TestLoginLimiterSweepDropsStaleEntries(t *testing.T) {
	l, advance := newClockedLimiter(t)
	failN(l, "10.0.0.1", "a@example.com", 1)
	failN(l, "10.0.0.2", "b@example.com", 1)

	advance(accountForget + sweepEvery + time.Second)
	l.RecordFailure("10.0.0.3", "c@example.com") // triggers the sweep

	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.pairs) != 1 || len(l.ips) != 1 || len(l.accounts) != 1 {
		t.Fatalf("want only the new failure kept, got %d pairs, %d ips, %d accounts",
			len(l.pairs), len(l.ips), len(l.accounts))
	}
}
