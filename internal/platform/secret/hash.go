package secret

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hash is how a random token is stored: the hex SHA-256 of it.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
