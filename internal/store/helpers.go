package store

import (
	"encoding/base64"
	"errors"
)

var (
	ErrWrongPassword = errors.New("wrong password")
	ErrTampered      = errors.New("vault data has been tampered with")
	ErrLocked        = errors.New("vault is locked")
	ErrReadOnly      = errors.New("vault is in read-only mode")
	ErrNotFound      = errors.New("not found")
	ErrTOTPRequired  = errors.New("TOTP code required")
	ErrTOTPInvalid   = errors.New("invalid TOTP code")
	ErrDecryptFailed = errors.New("encrypted content corrupted — AES-GCM authentication failed")
)

func encodeBytes(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func decodeBytes(encoded string) []byte {
	data, _ := base64.StdEncoding.DecodeString(encoded)
	return data
}
