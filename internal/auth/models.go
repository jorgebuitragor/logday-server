package auth

import "time"

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
}

// deviceWithOwner is a device joined with its owning user's email, for
// the admin panel's cross-user device list (listAllDevices) — device
// alone only has UserID, not something displayable.
type deviceWithOwner struct {
	device
	OwnerEmail string
}
