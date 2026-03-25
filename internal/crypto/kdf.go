package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	SaltSize   = 32
	KeySize    = 32 // AES-256
	NonceSize  = 12 // GCM nonce
	Iterations = 600000
)

// GenerateSalt generates a cryptographically random salt.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// DeriveKey derives a 256-bit AES key from a password and salt using PBKDF2-SHA256.
func DeriveKey(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, Iterations, KeySize, sha256.New)
}

// HashPassword derives a verifiable password hash using PBKDF2-SHA256.
// The result is base64-encoded and suitable for storage.
func HashPassword(password string, salt []byte) string {
	hash := pbkdf2.Key([]byte(password), salt, Iterations, KeySize, sha256.New)
	return base64.StdEncoding.EncodeToString(hash)
}
