package calendar

import "time"

var validColors = map[string]bool{
	"indigo": true, "amber": true, "emerald": true,
	"rose": true, "sky": true, "violet": true,
}

var validRepeats = map[string]bool{
	"none": true, "daily": true, "weekly": true,
	"biweekly": true, "monthly": true, "yearly": true,
}

// Event is exported because internal/sync reads rows directly (via
// ChangesSince) to build the unified /sync/changes feed.
type Event struct {
	ID              string     `json:"id"`
	UserID          string     `json:"-"`
	Title           string     `json:"title"`
	Date            string     `json:"date"`
	Time            string     `json:"time"`
	Description     string     `json:"description"`
	Color           string     `json:"color"`
	ReminderMinutes int        `json:"reminder_minutes"`
	Repeat          string     `json:"repeat"`
	Seq             int64      `json:"seq"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}
