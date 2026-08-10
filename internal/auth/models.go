package auth

import (
	"strings"
	"time"
)

type user struct {
	ID           string
	Email        string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    time.Time
	DeletedAt    *time.Time
}

type device struct {
	ID                    string
	UserID                string
	DeviceName            string
	RefreshTokenHash      string
	RefreshTokenExpiresAt time.Time
	CreatedAt             time.Time
	LastUsedAt            time.Time
	LastIP                string
	UserAgent             string
}

// deviceWithOwner is a device joined with its owning user's email, for
// the admin panel's cross-user device list (listAllDevices) — device
// alone only has UserID, not something displayable.
type deviceWithOwner struct {
	device
	OwnerEmail string
}

// IconName classifies a device from its recorded User-Agent for the
// admin panel's device-type icon — there's no structured device-type
// field anywhere in this codebase (DeviceName is arbitrary free text the
// client itself picks), so User-Agent substrings are the only real signal
// available. Defaults to "laptop" (matches the Logday desktop app and
// plain browser access, the expected majority case) whenever nothing
// more specific matches, including an empty User-Agent (devices created
// before this field existed).
func (d device) IconName() string {
	ua := strings.ToLower(d.UserAgent)
	switch {
	case strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet"):
		return "tablet"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "android") || strings.Contains(ua, "mobile"):
		return "smartphone"
	case strings.Contains(ua, "postman") || strings.Contains(ua, "insomnia") ||
		strings.Contains(ua, "curl") || strings.Contains(ua, "python-requests") || strings.Contains(ua, "httpie"):
		return "terminal"
	default:
		return "laptop"
	}
}
