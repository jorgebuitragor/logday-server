package security

import "github.com/alexedwards/argon2id"

// MaxPasswordLength is the longest password accepted, in bytes
// (specs/limites-auth). argon2id hashes any length in the same time, so
// this isn't about CPU: it keeps the limit explicit instead of depending
// on the request body limit, and 1024 bytes fits any passphrase or
// password manager output.
const MaxPasswordLength = 1024

// HashPassword hashes a plaintext password with argon2id, returning a
// self-describing PHC-format string (safe to store as-is).
func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

// VerifyPassword reports whether password matches the given argon2id hash.
func VerifyPassword(password, hash string) (bool, error) {
	match, _, err := argon2id.CheckHash(password, hash)
	return match, err
}
