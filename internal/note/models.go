package note

import "time"

// Note is exported because internal/sync reads note rows directly
// (via ChangesSince) to build the unified /sync/changes feed — it's
// also the JSON shape returned by this package's own REST endpoints.
//
// Content is a plain LWW-by-row field for now, same simplification as
// task.Content — it becomes CRDT-backed once the yrs/CGO integration
// is built (see specs/arquitectura-inicial, "Resolución de
// conflictos"). Not implemented yet: tracked as a follow-up.
type Note struct {
	ID        string     `json:"id"`
	UserID    string     `json:"-"`
	Title     string     `json:"title"`
	Folder    string     `json:"folder"`
	Tags      []string   `json:"tags"`
	Created   string     `json:"created"`
	Updated   string     `json:"updated"`
	Pinned    bool       `json:"pinned"`
	Content   string     `json:"content"`
	Seq       int64      `json:"seq"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
