package task

import "time"

var validStatuses = map[string]bool{
	"todo":        true,
	"in-progress": true,
	"done":        true,
}

type task struct {
	ID          string
	UserID      string
	Title       string
	TaskCode    *string
	Status      string
	Tags        []string
	Project     string
	Created     string
	CompletedAt *string
	Due         *string
	Content     string
	Seq         int64
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}
