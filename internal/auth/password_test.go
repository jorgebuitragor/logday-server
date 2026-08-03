package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}

	match, err := verifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("verifyPassword: %v", err)
	}
	if !match {
		t.Fatal("expected correct password to match")
	}

	match, err = verifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("verifyPassword: %v", err)
	}
	if match {
		t.Fatal("expected wrong password not to match")
	}
}
