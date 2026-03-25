package store

import (
	"encoding/json"
	"fmt"
	"os"

	"secretvault/internal/crypto"
	"secretvault/internal/signature"
)

// migrateV1toV2 upgrades a version-1 vault (password-based encryption)
// to version-2 (envelope encryption with DEK/KEK).
// It decrypts everything with the old password-based scheme, generates a
// DEK, re-encrypts with the DEK, wraps the DEK with a password-derived KEK,
// and generates a new recovery key that wraps the DEK as well.
//
// Returns the new recovery key that the caller must present to the user.
func migrateV1toV2(vf *VaultFile, password string) (recoveryKey string, err error) {
	salt := decodeBytes(vf.PasswordSalt)

	// ── 1. Decrypt private key using old password-based scheme ──
	privPEM, err := crypto.Decrypt(vf.PrivateKeyEnc, password)
	if err != nil {
		return "", fmt.Errorf("v1 migration: failed to decrypt private key: %w", err)
	}

	// ── 2. Decrypt vault data using old password-based scheme ──
	dataJSON, err := crypto.Decrypt(vf.EncryptedData, password)
	if err != nil {
		return "", fmt.Errorf("v1 migration: failed to decrypt vault data: %w", err)
	}

	// ── 3. Generate fresh DEK ──
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return "", err
	}

	// ── 4. Re-encrypt private key with DEK ──
	newPrivKeyEnc, err := crypto.EncryptWithKey(privPEM, dek)
	if err != nil {
		return "", err
	}

	// ── 5. Re-encrypt vault data with DEK ──
	newDataEnc, err := crypto.EncryptWithKey(dataJSON, dek)
	if err != nil {
		return "", err
	}

	// ── 6. Re-sign encrypted data ──
	privKey, err := signature.ImportPrivateKey(string(privPEM))
	if err != nil {
		return "", err
	}
	newSig, err := signature.Sign([]byte(newDataEnc), privKey)
	if err != nil {
		return "", err
	}

	// ── 7. Wrap DEK with KEK (password-derived) ──
	kek := crypto.DeriveKey(password, salt)
	dekEnc, err := crypto.EncryptWithKey(dek, kek)
	if err != nil {
		return "", err
	}

	// ── 8. Generate recovery key and wrap DEK ──
	recoveryKey, err = GenerateRecoveryKey()
	if err != nil {
		return "", err
	}
	rkSalt, err := crypto.GenerateSalt()
	if err != nil {
		return "", err
	}
	rkHash := HashRecoveryKey(recoveryKey, rkSalt)
	rkKEK := crypto.DeriveKey(NormalizeRecoveryKey(recoveryKey), rkSalt)
	recoveryDEKEnc, err := crypto.EncryptWithKey(dek, rkKEK)
	if err != nil {
		return "", err
	}

	// ── 9. Re-encrypt TOTP secret if present ──
	if vf.TOTPSecret != "" && vf.TOTPEnabled {
		totpPlain, decErr := crypto.Decrypt(vf.TOTPSecret, password)
		if decErr == nil {
			newTOTPEnc, encErr := crypto.EncryptWithKey(totpPlain, dek)
			if encErr == nil {
				vf.TOTPSecret = newTOTPEnc
			}
		}
	}

	// ── 10. Update vault file fields ──
	vf.Version = 2
	vf.PrivateKeyEnc = newPrivKeyEnc
	vf.EncryptedData = newDataEnc
	vf.Signature = newSig
	vf.DEKEnc = dekEnc
	vf.RecoveryKeySalt = encodeBytes(rkSalt)
	vf.RecoveryKeyHash = rkHash
	vf.RecoveryDEKEnc = recoveryDEKEnc

	// ── 10b. Compute integrity HMAC ──
	vf.Integrity = computeIntegrity(vf, dek)

	// ── 11. Persist to disk ──
	raw, err := json.MarshalIndent(vf, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(GetVaultPath(), raw, 0600); err != nil {
		return "", err
	}

	return recoveryKey, nil
}
