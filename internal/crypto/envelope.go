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

// ----- File Header (Internal ID) -----
// File format: [1 byte id_len][id_bytes][base64-encoded nonce+ciphertext+tag]
// The header stores the file's UUID so a file can be identified even if renamed.

// EncryptFileWithHeader encrypts plaintext with DEK and prepends a binary header
// containing the file's internal ID. The on-disk format is:
//
//	[1 byte: len(id)] [id bytes] [base64(nonce || ciphertext || tag)]
func EncryptFileWithHeader(plaintext, key []byte, fileID string) ([]byte, error) {
	encrypted, err := EncryptWithKey(plaintext, key)
	if err != nil {
		return nil, err
	}
	return prependHeader([]byte(encrypted), fileID), nil
}

// DecryptFileWithHeader reads the header to extract the internal ID, then
// decrypts the remaining payload with the given key.
func DecryptFileWithHeader(data, key []byte) (plaintext []byte, internalID string, err error) {
	id, payload, err := StripHeader(data)
	if err != nil {
		return nil, "", err
	}
	dec, err := DecryptWithKey(string(payload), key)
	if err != nil {
		return nil, id, err
	}
	return dec, id, nil
}

// ReadHeaderID reads only the internal ID from an encrypted file's header
// without decrypting. Returns ("", err) if the header is missing or malformed.
func ReadHeaderID(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty file")
	}
	idLen := int(data[0])
	if idLen == 0 || len(data) < 1+idLen {
		return "", errors.New("invalid or missing file header")
	}
	return string(data[1 : 1+idLen]), nil
}

// prependHeader creates [1 byte len][id bytes][payload].
func prependHeader(payload []byte, fileID string) []byte {
	id := []byte(fileID)
	if len(id) > 255 {
		id = id[:255]
	}
	out := make([]byte, 0, 1+len(id)+len(payload))
	out = append(out, byte(len(id)))
	out = append(out, id...)
	out = append(out, payload...)
	return out
}

// PrependHeaderToBase64 prepends the internal ID header to an already-encrypted
// base64 payload. Used by ImportFile when the ciphertext is already produced
// by EncryptWithKey.
func PrependHeaderToBase64(base64Payload []byte, fileID string) []byte {
	return prependHeader(base64Payload, fileID)
}

// StripHeader removes the header and returns (id, payload, err).
func StripHeader(data []byte) (string, []byte, error) {
	if len(data) == 0 {
		return "", nil, errors.New("empty file")
	}
	idLen := int(data[0])
	if idLen == 0 || len(data) < 1+idLen {
		return "", nil, errors.New("invalid or missing file header")
	}
	id := string(data[1 : 1+idLen])
	payload := data[1+idLen:]
	return id, payload, nil
}

// HasHeader returns true if the data starts with a valid header whose first
// byte is a plausible UUID length (36 bytes for standard UUIDs).
func HasHeader(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	idLen := int(data[0])
	// Standard UUID is 36 chars (8-4-4-4-12). Accept 32-40 range.
	return idLen >= 32 && idLen <= 40 && len(data) > 1+idLen
}

// EnsureFileHasHeader is a helper for migration: if a file is in the old
// format (plain base64, no header), it returns false. The caller can then
// re-write the file with a header.
func EnsureFileHasHeader(data []byte) bool {
	return HasHeader(data)
}

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
