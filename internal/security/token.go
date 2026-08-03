package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned by ParseJWT for a token that fails
// signature verification, is malformed, or is expired.
var ErrInvalidToken = errors.New("invalid token")

// GenerateOpaqueToken returns a new high-entropy random token: raw is
// the value handed to the caller, hash is what should be persisted for
// later lookup/verification (never store raw).
func GenerateOpaqueToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generating token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashOpaqueToken(raw), nil
}

// HashOpaqueToken hashes an opaque token for storage/lookup. SHA-256 is
// sufficient for already-high-entropy random values (unlike passwords,
// they're not something an attacker can feasibly brute force).
func HashOpaqueToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// SignJWT signs claims with secret using HS256.
func SignJWT(secret []byte, claims jwt.Claims) (string, error) {
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}

// ParseJWT verifies token against secret and decodes its claims into
// claims (a pointer to a jwt.Claims implementation).
func ParseJWT(secret []byte, token string, claims jwt.Claims) error {
	parsed, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (interface{}, error) {
		return secret, nil
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if !parsed.Valid {
		return ErrInvalidToken
	}
	return nil
}
