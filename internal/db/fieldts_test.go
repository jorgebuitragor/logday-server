package db

import (
	"testing"
	"time"
)

func TestFieldTimestampsWins(t *testing.T) {
	ft := FieldTimestamps{}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	if !ft.Wins("title", t0) {
		t.Fatal("first write to an untouched field should always win")
	}
	if ft.Wins("title", t0) {
		t.Fatal("a write with the same timestamp as the stored one should not win")
	}
	if !ft.Wins("title", t1) {
		t.Fatal("a strictly newer write should win")
	}
	if ft.Wins("title", t0) {
		t.Fatal("an older write than what's now stored should not win")
	}

	// A different field has its own independent timestamp.
	if !ft.Wins("status", t0) {
		t.Fatal("a different field should be independent of title's timestamp")
	}
}

func TestFieldTimestampsEncodeRoundTrip(t *testing.T) {
	ft := FieldTimestamps{"title": time.Date(2026, 1, 1, 12, 30, 0, 0, time.UTC)}

	encoded, err := ft.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := ParseFieldTimestamps(encoded)
	if err != nil {
		t.Fatalf("ParseFieldTimestamps: %v", err)
	}
	if !decoded["title"].Equal(ft["title"]) {
		t.Fatalf("round trip: got %v, want %v", decoded["title"], ft["title"])
	}
}

func TestParseFieldTimestampsEmpty(t *testing.T) {
	ft, err := ParseFieldTimestamps("{}")
	if err != nil {
		t.Fatalf("ParseFieldTimestamps: %v", err)
	}
	if len(ft) != 0 {
		t.Fatalf("got %d entries, want 0", len(ft))
	}
}
