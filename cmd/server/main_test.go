package main

import (
	"testing"
	"time"

	"github.com/jorgebuitragor/logday-server/internal/security"
)

func TestParseBodyLimit(t *testing.T) {
	cases := map[string]int64{
		"":     security.DefaultBodyLimitBytes,
		"  ":   security.DefaultBodyLimitBytes,
		"1":    1 << 20,
		" 64 ": 64 << 20,
		"1024": 1024 << 20,
	}
	for raw, want := range cases {
		got, err := parseBodyLimit(raw)
		if err != nil || got != want {
			t.Errorf("parseBodyLimit(%q) = %d, %v; want %d", raw, got, err, want)
		}
	}
	for _, bad := range []string{"0", "-5", "1025", "16MB", "1.5"} {
		if _, err := parseBodyLimit(bad); err == nil {
			t.Errorf("parseBodyLimit(%q): want an error", bad)
		}
	}
}

func TestNewHTTPServerSetsTimeouts(t *testing.T) {
	s := newHTTPServer(":0", nil)
	if s.ReadHeaderTimeout != 5*time.Second || s.ReadTimeout != time.Minute ||
		s.WriteTimeout != 2*time.Minute || s.IdleTimeout != 2*time.Minute {
		t.Fatalf("unexpected timeouts: %+v", s)
	}
}

func TestParseCORSOrigins(t *testing.T) {
	got := parseCORSOrigins(" https://a.example , ,https://b.example,")
	if len(got) != 2 || got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Fatalf("got %v", got)
	}
	if parseCORSOrigins("") != nil {
		t.Fatal("want nil for an empty value")
	}
}
