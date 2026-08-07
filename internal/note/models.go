package note

import "time"

// Note is exported because internal/sync reads note rows directly
// (via ChangesSince) to build the unified /sync/changes feed — it's
// also the JSON shape returned by this package's own REST endpoints.
//
// Content is CRDT-backed (github.com/Deln0r/ygo, wire-compatible with
// Yjs) via a dedicated endpoint (POST /notes/:id/content), separate
// from the LWW-by-row fields below — see specs/arquitectura-inicial,
// "Resolución de conflictos". ContentCRDT is the raw stored state
// (never serialized directly); Content/ContentState are computed from
// it for API responses.
type Note struct {
	ID           string     `json:"id"`
	UserID       string     `json:"-"`
	Title        string     `json:"title"`
	Folder       string     `json:"folder"`
	Tags         []string   `json:"tags"`
	Created      string     `json:"created"`
	Updated      string     `json:"updated"`
	Pinned       bool       `json:"pinned"`
	ContentCRDT  []byte     `json:"-"`
	Content      string     `json:"content"`
	ContentState string     `json:"content_state,omitempty"`
	Seq          int64      `json:"seq"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}
