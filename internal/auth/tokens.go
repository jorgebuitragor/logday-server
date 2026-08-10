package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jorgebuitragor/logday-server/internal/security"
)

type claims struct {
	UserID   string `json:"sub"`
	DeviceID string `json:"device_id"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

// issueAccessToken signs an access token valid for ttl — callers fetch
// ttl from internal/settings (Settings.AccessTokenTTL()) live on every
// login/refresh instead of a fixed constant, so an operator can change it
// without restarting the server.
func issueAccessToken(secret []byte, userID, deviceID string, isAdmin bool, ttl time.Duration) (string, error) {
	now := time.Now()
	c := claims{
		UserID:   userID,
		DeviceID: deviceID,
		IsAdmin:  isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
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
