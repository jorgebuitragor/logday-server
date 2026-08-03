package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	secret := []byte("test-secret")

	token, err := issueAccessToken(secret, "user-1", "device-1", true)
	if err != nil {
		t.Fatalf("issueAccessToken: %v", err)
	}

	c, err := parseAccessToken(secret, token)
	if err != nil {
		t.Fatalf("parseAccessToken: %v", err)
	}
	if c.UserID != "user-1" || c.DeviceID != "device-1" || !c.IsAdmin {
		t.Fatalf("unexpected claims: %+v", c)
	}
}

func TestParseAccessTokenRejectsWrongSecret(t *testing.T) {
	token, err := issueAccessToken([]byte("secret-a"), "user-1", "device-1", false)
	if err != nil {
		t.Fatalf("issueAccessToken: %v", err)
	}

	if _, err := parseAccessToken([]byte("secret-b"), token); err == nil {
		t.Fatal("expected error parsing token signed with a different secret")
	}
}

func TestParseAccessTokenRejectsExpired(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Now().Add(-accessTokenTTL - time.Minute)
	c := claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
	if err != nil {
		t.Fatalf("signing expired token: %v", err)
	}

	_, err = parseAccessToken(secret, token)
	if err == nil {
		t.Fatal("expected error parsing expired token")
	}
	if !errors.Is(err, errInvalidToken) {
		t.Fatalf("expected errInvalidToken, got %v", err)
	}
}

func TestGenerateRefreshTokenIsUniqueAndHashMatches(t *testing.T) {
	rawA, hashA, err := generateRefreshToken()
	if err != nil {
		t.Fatalf("generateRefreshToken: %v", err)
	}
	rawB, hashB, err := generateRefreshToken()
	if err != nil {
		t.Fatalf("generateRefreshToken: %v", err)
	}

	if rawA == rawB {
		t.Fatal("expected two independently generated tokens to differ")
	}
	if hashRefreshToken(rawA) != hashA {
		t.Fatal("hash does not match the raw token it was derived from")
	}
	if hashA == hashB {
		t.Fatal("expected hashes of different tokens to differ")
	}
}
