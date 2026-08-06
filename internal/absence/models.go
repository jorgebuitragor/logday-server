package absence

import "time"

var validTypes = map[string]bool{
	"incapacidad": true,
	"vacaciones":  true,
	"otro":        true,
}

// Day is exported because internal/sync reads rows directly (via
// ChangesSince) to build the unified /sync/changes feed.
type Day struct {
	ID        string     `json:"id"`
	UserID    string     `json:"-"`
	Date      string     `json:"date"`
	Type      string     `json:"type"`
	Note      *string    `json:"note,omitempty"`
	Seq       int64      `json:"seq"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
