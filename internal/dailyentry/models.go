package dailyentry

import "time"

// Entry is keyed by (user_id, date) — no client-generated id, same
// shape as overtime.MonthMeta. Date acts as the synthetic id in REST
// URLs and sync changes.
//
// Content is a plain LWW-by-row field for now, same simplification as
// note.Content — it becomes CRDT-backed once the yrs/CGO integration
// is built (see specs/arquitectura-inicial, "Resolución de
// conflictos"). Not implemented yet: tracked as a follow-up.
type Entry struct {
	UserID    string     `json:"-"`
	Date      string     `json:"date"`
	Content   string     `json:"content"`
	Seq       int64      `json:"seq"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
