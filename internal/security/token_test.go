package security

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSignAndParseJWTRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	claims := jwt.RegisteredClaims{
		Subject:   "user-1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}

	token, err := SignJWT(secret, claims)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	got := &jwt.RegisteredClaims{}
	if err := ParseJWT(secret, token, got); err != nil {
		t.Fatalf("ParseJWT: %v", err)
	}
	if got.Subject != "user-1" {
		t.Fatalf("unexpected subject: %q", got.Subject)
	}
}

func TestParseJWTRejectsWrongSecret(t *testing.T) {
	claims := jwt.RegisteredClaims{Subject: "user-1"}
	token, err := SignJWT([]byte("secret-a"), claims)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	if err := ParseJWT([]byte("secret-b"), token, &jwt.RegisteredClaims{}); err == nil {
		t.Fatal("expected error parsing token signed with a different secret")
	}
}

func TestParseJWTRejectsExpired(t *testing.T) {
	secret := []byte("test-secret")
	past := time.Now().Add(-time.Hour)
	claims := jwt.RegisteredClaims{
		Subject:   "user-1",
		IssuedAt:  jwt.NewNumericDate(past),
		ExpiresAt: jwt.NewNumericDate(past.Add(time.Minute)),
	}
	token, err := SignJWT(secret, claims)
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	err = ParseJWT(secret, token, &jwt.RegisteredClaims{})
	if err == nil {
		t.Fatal("expected error parsing expired token")
	}
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestGenerateOpaqueTokenIsUniqueAndHashMatches(t *testing.T) {
	rawA, hashA, err := GenerateOpaqueToken()
	if err != nil {
		t.Fatalf("GenerateOpaqueToken: %v", err)
	}
	rawB, hashB, err := GenerateOpaqueToken()
	if err != nil {
		t.Fatalf("GenerateOpaqueToken: %v", err)
	}

	if rawA == rawB {
		t.Fatal("expected two independently generated tokens to differ")
	}
	if HashOpaqueToken(rawA) != hashA {
		t.Fatal("hash does not match the raw token it was derived from")
	}
	if hashA == hashB {
		t.Fatal("expected hashes of different tokens to differ")
	}
}
