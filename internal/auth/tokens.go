package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jorgebuitragor/logday-server/internal/security"
)

const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 30 * 24 * time.Hour
)

type claims struct {
	UserID   string `json:"sub"`
	DeviceID string `json:"device_id"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

func issueAccessToken(secret []byte, userID, deviceID string, isAdmin bool) (string, error) {
	now := time.Now()
	c := claims{
		UserID:   userID,
		DeviceID: deviceID,
		IsAdmin:  isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
		},
	}
	return security.SignJWT(secret, c)
}

func parseAccessToken(secret []byte, tokenString string) (*claims, error) {
	c := &claims{}
	if err := security.ParseJWT(secret, tokenString, c); err != nil {
		return nil, err
	}
	return c, nil
}
