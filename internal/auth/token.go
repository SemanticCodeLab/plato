// Package auth handles Plato's agent-style API tokens. Tokens are opaque random
// strings prefixed with "plato_"; only their SHA-256 hash is ever stored.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Prefix is prepended to every raw token.
const Prefix = "plato_"

// Scopes.
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
)

// Generate returns a new raw token (with prefix) and its hash for storage.
func Generate() (raw, hash string) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic("auth: cannot read random bytes: " + err.Error())
	}
	raw = Prefix + hex.EncodeToString(b)
	return raw, Hash(raw)
}

// Hash returns the hex SHA-256 of a raw token.
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// HasScope reports whether a comma-separated scope string grants want.
func HasScope(scopes, want string) bool {
	for _, s := range strings.Split(scopes, ",") {
		if strings.TrimSpace(s) == want {
			return true
		}
	}
	return false
}

// NormalizeScopes validates and canonicalizes a scope list. Unknown scopes are
// dropped. If write is granted, read is implied. Defaults to read when empty.
func NormalizeScopes(in []string) string {
	read, write := false, false
	for _, s := range in {
		switch strings.TrimSpace(s) {
		case ScopeRead:
			read = true
		case ScopeWrite:
			write = true
		}
	}
	if write {
		return "read,write"
	}
	if read {
		return "read"
	}
	return "read"
}
