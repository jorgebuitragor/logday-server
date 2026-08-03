package auth

import "time"

type user struct {
	ID           string
	Email        string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    time.Time
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
