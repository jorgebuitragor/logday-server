// Package crdt wraps github.com/Deln0r/ygo (a pure-Go, wire-compatible
// port of Yjs) into the two operations this project's domain packages
// actually need for a CRDT-backed text field: merge an incoming
// client update into stored state, and decode stored state into plain
// text. No domain knowledge here — same rule as internal/security:
// this package doesn't know what a Note or a daily entry is.
package crdt

import (
	"fmt"

	"github.com/Deln0r/ygo"
)

// textKey names the shared Text type inside every doc this package
// creates. Only one text field per doc is needed for this project's
// use case (Note.Content, daily entry Content), so a fixed name is
// fine — it never appears outside this package.
const textKey = "content"

// ApplyTextUpdate merges update into the CRDT state, returning the
// new compacted state (safe to persist as-is — Yjs updates are
// idempotent, so replaying it is harmless) and the resulting plain
// text. state may be nil/empty, meaning "no document yet".
func ApplyTextUpdate(state, update []byte) (newState []byte, text string, err error) {
	doc := ygo.NewDoc()
	t := ygo.NewText(doc, textKey)

	if len(state) > 0 {
		if err := ygo.ApplyUpdate(doc, state); err != nil {
			return nil, "", fmt.Errorf("loading existing CRDT state: %w", err)
		}
	}
	if err := ygo.ApplyUpdate(doc, update); err != nil {
		return nil, "", fmt.Errorf("applying CRDT update: %w", err)
	}

	return ygo.EncodeStateAsUpdate(doc), t.String(), nil
}

// Text decodes the plain text from stored CRDT state. state may be
// nil/empty, meaning "no document yet" (returns "").
func Text(state []byte) (string, error) {
	if len(state) == 0 {
		return "", nil
	}
	doc := ygo.NewDoc()
	t := ygo.NewText(doc, textKey)
	if err := ygo.ApplyUpdate(doc, state); err != nil {
		return "", fmt.Errorf("loading CRDT state: %w", err)
	}
	return t.String(), nil
}
