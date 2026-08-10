package auth

import (
	"testing"
	"time"
)

// Round-trip of the auth-specific claims (UserID/DeviceID/IsAdmin).
// Generic JWT signing/parsing/expiry behavior is covered in
// internal/security, which this wraps.
func TestAccessTokenRoundTrip(t *testing.T) {
	secret := []byte("test-secret")

	token, err := issueAccessToken(secret, "user-1", "device-1", true, 15*time.Minute)
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
