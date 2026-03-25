package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"secretvault/internal/crypto"
	"secretvault/internal/signature"
)

// GenerateRecoveryKey creates a random 24-character hex recovery key
// formatted as groups of 4 for readability: XXXX-XXXX-XXXX-XXXX-XXXX-XXXX
func GenerateRecoveryKey() (string, error) {
	b := make([]byte, 12) // 12 bytes = 24 hex chars
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	h := strings.ToUpper(hex.EncodeToString(b))
	// Format as XXXX-XXXX-XXXX-XXXX-XXXX-XXXX
	parts := make([]string, 6)
	for i := 0; i < 6; i++ {
		parts[i] = h[i*4 : (i+1)*4]
	}
	return strings.Join(parts, "-"), nil
}

// NormalizeRecoveryKey strips dashes and converts to uppercase for comparison
func NormalizeRecoveryKey(key string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(key), "-", ""))
}

// HashRecoveryKey hashes a recovery key with a salt using SHA-256
// (separate from password hashing to keep them independent)
func HashRecoveryKey(key string, salt []byte) string {
	normalized := NormalizeRecoveryKey(key)
	data := append(salt, []byte(normalized)...)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ValidateRecoveryKey checks whether the given recovery key matches the stored hash
func (s *Store) ValidateRecoveryKey(recoveryKey string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := GetVaultPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file VaultFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return err
	}

	if file.RecoveryKeySalt == "" || file.RecoveryKeyHash == "" {
		return fmt.Errorf("no recovery key configured for this vault")
	}

	salt := decodeBytes(file.RecoveryKeySalt)
	hash := HashRecoveryKey(recoveryKey, salt)
	if hash != file.RecoveryKeyHash {
		return fmt.Errorf("invalid recovery key")
	}

	return nil
}

// ResetPasswordWithRecovery validates the recovery key, unwraps the DEK,
// then re-wraps it with a new password-derived KEK. No data re-encryption needed.
func (s *Store) ResetPasswordWithRecovery(recoveryKey, newPassword string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := GetVaultPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var vf VaultFile
	if err := json.Unmarshal(raw, &vf); err != nil {
		return "", err
	}
	file := &vf

	// Validate recovery key
	if file.RecoveryKeySalt == "" || file.RecoveryKeyHash == "" {
		return "", fmt.Errorf("no recovery key configured for this vault")
	}
	rkSalt := decodeBytes(file.RecoveryKeySalt)
	rkHash := HashRecoveryKey(recoveryKey, rkSalt)
	if rkHash != file.RecoveryKeyHash {
		return "", fmt.Errorf("invalid recovery key")
	}

	// Derive key from recovery key and unwrap DEK
	if file.RecoveryDEKEnc == "" {
		return "", fmt.Errorf("recovery data is incomplete — cannot reset password")
	}
	rkKey := crypto.DeriveKey(NormalizeRecoveryKey(recoveryKey), rkSalt)
	dek, err := crypto.DecryptWithKey(file.RecoveryDEKEnc, rkKey)
	if err != nil {
		return "", fmt.Errorf("failed to unwrap DEK with recovery key: %w", err)
	}

	// Decrypt private key and data with DEK to verify + load into memory
	privPEM, err := crypto.DecryptWithKey(file.PrivateKeyEnc, dek)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt private key: %w", err)
	}
	privKey, err := signature.ImportPrivateKey(string(privPEM))
	if err != nil {
		return "", fmt.Errorf("failed to import private key: %w", err)
	}
	pubKey, err := signature.ImportPublicKey(file.PublicKeyPEM)
	if err != nil {
		return "", err
	}

	dataJSON, err := crypto.DecryptWithKey(file.EncryptedData, dek)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt vault data: %w", err)
	}
	var data VaultData
	if err := json.Unmarshal(dataJSON, &data); err != nil {
		return "", err
	}

	// Generate new password salt and hash
	newSalt, err := crypto.GenerateSalt()
	if err != nil {
		return "", err
	}
	newPassHash := crypto.HashPassword(newPassword, newSalt)

	// Derive new KEK and re-wrap the existing DEK
	newKEK := crypto.DeriveKey(newPassword, newSalt)
	newDEKEnc, err := crypto.EncryptWithKey(dek, newKEK)
	if err != nil {
		return "", err
	}

	// Generate new recovery key and re-wrap DEK
	newRecoveryKey, err := GenerateRecoveryKey()
	if err != nil {
		return "", err
	}
	newRKSalt, err := crypto.GenerateSalt()
	if err != nil {
		return "", err
	}
	newRKHash := HashRecoveryKey(newRecoveryKey, newRKSalt)
	newRKKey := crypto.DeriveKey(NormalizeRecoveryKey(newRecoveryKey), newRKSalt)
	newDEKRecoveryEnc, err := crypto.EncryptWithKey(dek, newRKKey)
	if err != nil {
		return "", err
	}

	// Update vault file — only password-related and recovery-related fields change
	file.PasswordSalt = encodeBytes(newSalt)
	file.PasswordHash = newPassHash
	file.DEKEnc = newDEKEnc
	file.RecoveryKeySalt = encodeBytes(newRKSalt)
	file.RecoveryKeyHash = newRKHash
	file.RecoveryDEKEnc = newDEKRecoveryEnc

	// Update store state — unlock the vault with the new password
	s.filePath = path
	s.password = newPassword
	s.dek = dek
	s.data = &data
	s.file = file
	s.keyPair = &signature.KeyPair{PrivateKey: privKey, PublicKey: pubKey}
	s.readOnly = false

	if err := s.saveFile(); err != nil {
		return "", err
	}

	return newRecoveryKey, nil
}
