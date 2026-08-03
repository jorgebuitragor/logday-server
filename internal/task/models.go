package task

import "time"

var validStatuses = map[string]bool{
	"todo":        true,
	"in-progress": true,
	"done":        true,
}

// Task is exported because internal/sync reads task rows directly (via
// ChangesSince) to build the unified /sync/changes feed — it's also
// the JSON shape returned by this package's own REST endpoints.
type Task struct {
	ID          string     `json:"id"`
	UserID      string     `json:"-"`
	Title       string     `json:"title"`
	TaskCode    *string    `json:"task_code,omitempty"`
	Status      string     `json:"status"`
	Tags        []string   `json:"tags"`
	Project     string     `json:"project"`
	Created     string     `json:"created"`
	CompletedAt *string    `json:"completed_at,omitempty"`
	Due         *string    `json:"due,omitempty"`
	Content     string     `json:"content"`
	Seq         int64      `json:"seq"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}
