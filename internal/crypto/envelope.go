package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// GenerateDEK generates a random 256-bit Data Encryption Key.
func GenerateDEK() ([]byte, error) {
	dek := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, err
	}
	return dek, nil
}

// EncryptWithKey encrypts plaintext using a raw AES-256-GCM key (no PBKDF2).
// Returns base64-encoded: nonce (12 B) || ciphertext+tag.
func EncryptWithKey(plaintext, key []byte) (string, error) {
	if len(key) != KeySize {
		return "", errors.New("invalid key size: expected 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Layout: nonce (12 B) || ciphertext+tag
	combined := make([]byte, 0, len(nonce)+len(ciphertext))
	combined = append(combined, nonce...)
	combined = append(combined, ciphertext...)

	return base64.StdEncoding.EncodeToString(combined), nil
}

// DecryptWithKey decrypts base64-encoded AES-256-GCM ciphertext using a raw key.
func DecryptWithKey(encoded string, key []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, errors.New("invalid key size: expected 32 bytes")
	}

	combined, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	if len(combined) < NonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce := combined[:NonceSize]
	ciphertext := combined[NonceSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("decryption failed: wrong key or corrupted data")
	}

	return plaintext, nil
}

// ComputeHMAC computes HMAC-SHA256 over the concatenation of the given fields
// using a derived HMAC key (SHA-256 of DEK || "hmac-integrity").
// Returns a hex-encoded string.
func ComputeHMAC(dek []byte, fields ...string) string {
	// Derive a separate HMAC key from DEK so we never use the DEK directly for MAC
	h := sha256.New()
	h.Write(dek)
	h.Write([]byte("hmac-integrity"))
	hmacKey := h.Sum(nil)

	mac := hmac.New(sha256.New, hmacKey)
	for i, f := range fields {
		if i > 0 {
			mac.Write([]byte("\x00")) // separator
		}
		mac.Write([]byte(f))
	}
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyHMAC checks that the given HMAC matches the expected value.
func VerifyHMAC(dek []byte, expected string, fields ...string) bool {
	computed := ComputeHMAC(dek, fields...)
	return strings.EqualFold(computed, expected)
}
