package auth

import (
	"context"
	"database/sql"
	"log"
	"strings"
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

// Limits on failed logins (specs/limites-auth). maxAttempts and window
// come from the panel's Configuración (instance_settings):
//
//   - Per IP and email: maxAttempts failures per window, then 429 until
//     the oldest one leaves the window. The original limit.
//   - Per IP, any email: ipAttemptsFactor × maxAttempts per window, so
//     one address can't spray passwords across many accounts.
//   - Per account (email), any IP: after maxAttempts failures each new
//     one makes the account wait, doubling from accountBaseDelay up to
//     accountMaxDelay. It's a delay and not a lockout on purpose: with a
//     lockout, anyone who knows an email could keep its owner out
//     forever. A guess from many IPs gets one try per accountMaxDelay at
//     most; the owner waits that long at worst.
//
// The account counter forgets after accountForget without failures, and
// a successful login clears the IP+email and account counters (not the
// IP one: a correct password for one account says nothing about the
// other guesses from that address).
const (
	ipAttemptsFactor = 4
	accountBaseDelay = time.Second
	accountMaxDelay  = 5 * time.Minute
	accountForget    = time.Hour
	sweepEvery       = time.Minute
)

// loginLimiter keeps its state in memory. That's enough for the target
// deployment of this project: a single self-hosted instance, not a
// multi-replica cluster. The limit/window themselves are read live from
// instance_settings on every check (a single indexed PK read — login
// attempts are nowhere near a hot path), so a change made in the admin
// panel's Configuración section takes effect immediately, no restart.
type loginLimiter struct {
	db  *sql.DB
	now func() time.Time // a field only so tests can move the clock

	mu        sync.Mutex
	pairs     map[string][]time.Time
	ips       map[string][]time.Time
	accounts  map[string]*accountFailures
	lastSweep time.Time
}

type accountFailures struct {
	count int
	last  time.Time
	until time.Time
}

func newLoginLimiter(db *sql.DB) *loginLimiter {
	return &loginLimiter{
		db:       db,
		now:      time.Now,
		pairs:    make(map[string][]time.Time),
		ips:      make(map[string][]time.Time),
		accounts: make(map[string]*accountFailures),
	}
}

func (l *loginLimiter) limits() (maxAttempts int, window time.Duration) {
	cfg, err := settings.Get(context.Background(), l.db)
	if err != nil {
		log.Printf("reading login rate limit settings, using fallback: %v", err)
		return fallbackMaxAttempts, fallbackWindow
	}
	return cfg.LoginRateLimitAttempts, cfg.LoginRateLimitWindow()
}

func accountKey(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func pairKey(ip, email string) string { return ip + "|" + accountKey(email) }

// Check reports how long the caller must wait before trying ip+email
// again: zero when the attempt is allowed.
func (l *loginLimiter) Check(ip, email string) time.Duration {
	maxAttempts, window := l.limits()
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()

	wait := windowWait(l.prune(l.pairs, pairKey(ip, email), now, window), maxAttempts, now, window)
	wait = max(wait, windowWait(l.prune(l.ips, ip, now, window), ipAttemptsFactor*maxAttempts, now, window))
	if a := l.accounts[accountKey(email)]; a != nil && a.until.After(now) {
		wait = max(wait, a.until.Sub(now))
	}
	return wait
}

// RecordFailure counts a failed attempt for ip+email.
func (l *loginLimiter) RecordFailure(ip, email string) {
	maxAttempts, window := l.limits()
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now, window)

	pk := pairKey(ip, email)
	l.pairs[pk] = append(l.prune(l.pairs, pk, now, window), now)
	l.ips[ip] = append(l.prune(l.ips, ip, now, window), now)

	ak := accountKey(email)
	a := l.accounts[ak]
	if a == nil || now.Sub(a.last) > accountForget {
		a = &accountFailures{}
		l.accounts[ak] = a
	}
	a.count++
	a.last = now
	if over := a.count - maxAttempts; over > 0 {
		a.until = now.Add(backoffDelay(over))
	}
}

// Reset clears the IP+email and account counters after a successful
// login.
func (l *loginLimiter) Reset(ip, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.pairs, pairKey(ip, email))
	delete(l.accounts, accountKey(email))
}

// backoffDelay is the wait after the n-th failure past the free ones:
// accountBaseDelay doubled n-1 times, capped at accountMaxDelay.
func backoffDelay(n int) time.Duration {
	d := accountBaseDelay
	for i := 1; i < n && d < accountMaxDelay; i++ {
		d *= 2
	}
	return min(d, accountMaxDelay)
}

// windowWait is how long until attempts (oldest first, all inside the
// window) drops below limit.
func windowWait(attempts []time.Time, limit int, now time.Time, window time.Duration) time.Duration {
	limit = max(limit, 1) // the panel enforces 1..100; never index past the slice
	if len(attempts) < limit {
		return 0
	}
	return attempts[len(attempts)-limit].Add(window).Sub(now)
}

// prune drops attempts that left the window, and the key once none is
// left, so the maps only hold recent failures.
func (l *loginLimiter) prune(m map[string][]time.Time, key string, now time.Time, window time.Duration) []time.Time {
	cutoff := now.Add(-window)
	kept := m[key][:0]
	for _, t := range m[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(m, key)
		return nil
	}
	m[key] = kept
	return kept
}

// sweep drops stale keys that nobody has checked again, so failures with
// made-up emails or from many addresses don't pile up in memory.
func (l *loginLimiter) sweep(now time.Time, window time.Duration) {
	if now.Sub(l.lastSweep) < sweepEvery {
		return
	}
	l.lastSweep = now
	for k := range l.pairs {
		l.prune(l.pairs, k, now, window)
	}
	for k := range l.ips {
		l.prune(l.ips, k, now, window)
	}
	for k, a := range l.accounts {
		if now.Sub(a.last) > accountForget && !a.until.After(now) {
			delete(l.accounts, k)
		}
	}
}
