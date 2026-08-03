package sync

import (
	"encoding/json"
	"time"
)

type change struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Seq       int64           `json:"seq"`
	Deleted   bool            `json:"deleted"`
	UpdatedAt time.Time       `json:"updated_at"`
	Data      json.RawMessage `json:"data"`
}
