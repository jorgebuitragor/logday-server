package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 30 * 24 * time.Hour
)

var errInvalidToken = errors.New("invalid token")

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
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("signing access token: %w", err)
	}
	return signed, nil
}

func parseAccessToken(secret []byte, tokenString string) (*claims, error) {
	c := &claims{}
	token, err := jwt.ParseWithClaims(tokenString, c, func(*jwt.Token) (interface{}, error) {
		return secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidToken, err)
	}
	if !token.Valid {
		return nil, errInvalidToken
	}
	return c, nil
}

// generateRefreshToken returns a new high-entropy refresh token: raw is
// the value handed to the client, hash is what gets persisted.
func generateRefreshToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generating refresh token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashRefreshToken(raw), nil
}

// hashRefreshToken hashes a refresh token for storage/lookup. SHA-256 is
// sufficient (unlike passwords, refresh tokens are already high-entropy
// random values, not something an attacker can feasibly brute force).
func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
