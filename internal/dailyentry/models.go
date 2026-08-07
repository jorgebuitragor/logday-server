package dailyentry

import "time"

// Entry is keyed by (user_id, date) — no client-generated id, same
// shape as overtime.MonthMeta. Date acts as the synthetic id in REST
// URLs and sync changes.
//
// Unlike Note, there are no LWW-governed fields alongside content —
// the entire entity is CRDT-backed text (github.com/Deln0r/ygo,
// wire-compatible with Yjs), so there's a single write path
// (applyContentUpdate) instead of a create/content split. ContentCRDT
// is the raw stored state (never serialized directly);
// Content/ContentState are computed from it for API responses.
type Entry struct {
	UserID       string     `json:"-"`
	Date         string     `json:"date"`
	ContentCRDT  []byte     `json:"-"`
	Content      string     `json:"content"`
	ContentState string     `json:"content_state,omitempty"`
	Seq          int64      `json:"seq"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}
