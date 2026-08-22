package db

import (
	"encoding/json"
	"fmt"
	"time"
)

// FieldTimestamps tracks, per field name, the updated_at of the write
// that last won LWW for that field — the mechanism behind PATCH-based
// per-field conflict resolution (see specs/lww-por-campo). A field
// absent from the map has never been explicitly written, so any
// incoming timestamp wins it unconditionally.
type FieldTimestamps map[string]time.Time

// ParseFieldTimestamps decodes a stored field_updated_at column. An
// empty string (shouldn't happen given the column's NOT NULL DEFAULT
// '{}', but cheap to handle) decodes as an empty map.
func ParseFieldTimestamps(raw string) (FieldTimestamps, error) {
	if raw == "" {
		return FieldTimestamps{}, nil
	}
	var m map[string]time.Time
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("decoding field_updated_at: %w", err)
	}
	if m == nil {
		m = map[string]time.Time{}
	}
	return FieldTimestamps(m), nil
}

// Encode serializes ft back to the form stored in field_updated_at.
func (ft FieldTimestamps) Encode() (string, error) {
	b, err := json.Marshal(map[string]time.Time(ft))
	if err != nil {
		return "", fmt.Errorf("encoding field_updated_at: %w", err)
	}
	return string(b), nil
}

// Wins reports whether incoming should overwrite field's current
// value — true if field was never written before, or its stored
// timestamp isn't newer than incoming. On a win, it records incoming
// as field's new timestamp; callers must still apply the value
// themselves (Wins only tracks the timestamp side of the merge).
func (ft FieldTimestamps) Wins(field string, incoming time.Time) bool {
	if existing, ok := ft[field]; ok && !incoming.After(existing) {
		return false
	}
	ft[field] = incoming
	return true
}
