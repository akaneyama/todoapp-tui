package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// tokenBytes is the amount of entropy in a session token.
const tokenBytes = 32

// NewToken returns a random URL-safe session token. Only its hash is ever
// persisted; the raw token is returned to the client exactly once.
func NewToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the SHA-256 digest stored in sessions.token_hash. A fast
// hash is fine here because the token is high-entropy random data, not a
// user-chosen secret.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
