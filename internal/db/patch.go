package db

import (
	"encoding/json"
	"fmt"
	"io"
)

// Field represents one key of a PATCH request body: whether it was
// present at all, and its decoded value if so. A plain pointer field
// can't distinguish "absent from the payload" from "present with an
// explicit null" — both decode to nil — which matters for nullable
// columns, where null means "clear this field" and absent means
// "leave it alone" (see specs/lww-por-campo).
type Field[T any] struct {
	Set   bool
	Value T
}

// ParsePatch decodes r into a map of raw JSON per top-level key, the
// input to PatchField.
func ParsePatch(r io.Reader) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding patch body: %w", err)
	}
	return raw, nil
}

// PatchField extracts key from raw into a Field[T]. Returns a zero
// Field (Set: false) if key is absent — never an error for that case,
// only for a key that's present but doesn't decode into T.
func PatchField[T any](raw map[string]json.RawMessage, key string) (Field[T], error) {
	v, ok := raw[key]
	if !ok {
		return Field[T]{}, nil
	}
	var value T
	if err := json.Unmarshal(v, &value); err != nil {
		return Field[T]{}, fmt.Errorf("decoding %q: %w", key, err)
	}
	return Field[T]{Set: true, Value: value}, nil
}
