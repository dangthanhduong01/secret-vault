package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"secretvault/internal/crypto"
	"secretvault/internal/signature"

	"github.com/google/uuid"
)

// Note represents a single encrypted note
type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"` // Markdown content
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FileMetadata holds metadata for an encrypted file
type FileMetadata struct {
	ID           string    `json:"id"`
	OriginalName string    `json:"original_name"`
	MimeType     string    `json:"mime_type"`
	Size         int64     `json:"size"`
	ContentHash  string    `json:"content_hash"` // SHA-256 of original file
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Signature    string    `json:"signature"` // ECDSA signature over metadata fields
	Tampered     bool      `json:"tampered,omitempty"`
}

// VaultData holds all decrypted vault data in memory
type VaultData struct {
	Notes []Note         `json:"notes"`
	Files []FileMetadata `json:"files"`
}

// VaultFile represents the encrypted file on disk
type VaultFile struct {
	Version         int    `json:"version"`
	PasswordSalt    string `json:"password_salt"`
	PasswordHash    string `json:"password_hash"`
	TOTPSecret      string `json:"totp_secret"`
	TOTPEnabled     bool   `json:"totp_enabled"`
	PrivateKeyEnc   string `json:"private_key_enc"` // ECDSA private key encrypted with DEK
	PublicKeyPEM    string `json:"public_key_pem"`
	EncryptedData   string `json:"encrypted_data"` // vault data encrypted with DEK
	Signature       string `json:"signature"`      // ECDSA signature of encrypted_data
	DEKEnc          string `json:"dek_enc"`        // DEK encrypted with KEK (password-derived)
	RecoveryKeySalt string `json:"recovery_key_salt,omitempty"`
	RecoveryKeyHash string `json:"recovery_key_hash,omitempty"`
	RecoveryDEKEnc  string `json:"recovery_dek_enc,omitempty"` // DEK encrypted with recovery key
	Integrity       string `json:"integrity,omitempty"`        // HMAC-SHA256 over critical fields
}

// Store manages the encrypted vault
type Store struct {
	mu                   sync.RWMutex
	filePath             string
	password             string
	dek                  []byte // Data Encryption Key (raw 32 bytes, in memory only)
	data                 *VaultData
	file                 *VaultFile
	keyPair              *signature.KeyPair
	readOnly             bool   // true when opened despite tamper detection failure
	migrationRecoveryKey string // set when v1→v2 migration creates a new recovery key
}

// NewStore creates a new store instance
func NewStore() *Store {
	return &Store{}
}

// GetVaultPath returns the default vault file path
func GetVaultPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".secretvault")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "vault.json")
}

// VaultExists checks if a vault file already exists
func (s *Store) VaultExists() bool {
	path := GetVaultPath()
	_, err := os.Stat(path)
	return err == nil
}

// CreateVault creates a new vault with the given password.
// Returns the recovery key that the user must save.
func (s *Store) CreateVault(password string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// --- Password hash ---
	salt, err := crypto.GenerateSalt()
	if err != nil {
		return "", err
	}
	saltB64 := encodeBytes(salt)
	passHash := crypto.HashPassword(password, salt)

	// --- DEK (Data Encryption Key) — random, never changes ---
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return "", err
	}

	// --- KEK (Key Encryption Key) — derived from password ---
	kek := crypto.DeriveKey(password, salt)

	// Wrap DEK with KEK
	dekEnc, err := crypto.EncryptWithKey(dek, kek)
	if err != nil {
		return "", err
	}

	// --- ECDSA key pair ---
	kp, err := signature.GenerateKeyPair()
	if err != nil {
		return "", err
	}
	privPEM, err := signature.ExportPrivateKey(kp.PrivateKey)
	if err != nil {
		return "", err
	}
	pubPEM, err := signature.ExportPublicKey(kp.PublicKey)
	if err != nil {
		return "", err
	}

	// Encrypt private key with DEK
	encPrivKey, err := crypto.EncryptWithKey([]byte(privPEM), dek)
	if err != nil {
		return "", err
	}

	// --- Vault data (empty) ---
	data := &VaultData{Notes: []Note{}, Files: []FileMetadata{}}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	// Encrypt vault data with DEK
	encData, err := crypto.EncryptWithKey(dataJSON, dek)
	if err != nil {
		return "", err
	}

	// Sign the encrypted data
	sig, err := signature.Sign([]byte(encData), kp.PrivateKey)
	if err != nil {
		return "", err
	}

	// --- Recovery key ---
	recoveryKey, err := GenerateRecoveryKey()
	if err != nil {
		return "", err
	}
	rkSalt, err := crypto.GenerateSalt()
	if err != nil {
		return "", err
	}
	rkHash := HashRecoveryKey(recoveryKey, rkSalt)

	// Wrap DEK with recovery key (so recovery can unwrap DEK without password)
	rkDEK := crypto.DeriveKey(NormalizeRecoveryKey(recoveryKey), rkSalt)
	recoveryDEKEnc, err := crypto.EncryptWithKey(dek, rkDEK)
	if err != nil {
		return "", err
	}

	vaultFile := &VaultFile{
		Version:         2,
		PasswordSalt:    saltB64,
		PasswordHash:    passHash,
		TOTPSecret:      "",
		TOTPEnabled:     false,
		PrivateKeyEnc:   encPrivKey,
		PublicKeyPEM:    pubPEM,
		EncryptedData:   encData,
		Signature:       sig,
		DEKEnc:          dekEnc,
		RecoveryKeySalt: encodeBytes(rkSalt),
		RecoveryKeyHash: rkHash,
		RecoveryDEKEnc:  recoveryDEKEnc,
	}

	s.filePath = GetVaultPath()
	s.password = password
	s.dek = dek
	s.data = data
	s.file = vaultFile
	s.keyPair = kp

	return recoveryKey, s.saveFile()
}

// Unlock unlocks an existing vault with password
func (s *Store) Unlock(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := GetVaultPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var vaultFile VaultFile
	if err := json.Unmarshal(raw, &vaultFile); err != nil {
		return err
	}

	// Verify password
	salt := decodeBytes(vaultFile.PasswordSalt)
	passHash := crypto.HashPassword(password, salt)
	if passHash != vaultFile.PasswordHash {
		return ErrWrongPassword
	}

	// ── Auto-migrate v1 → v2 (envelope encryption) ──
	if vaultFile.DEKEnc == "" {
		return ErrWrongPassword // refuse to open v1 vaults — user must explicitly migrate with the CLI tool
		// recoveryKey, err := migrateV1toV2(&vaultFile, password)
		// if err != nil {
		// 	return fmt.Errorf("vault migration failed: %w", err)
		// }
		// // Re-read the migrated file
		// raw2, err := os.ReadFile(path)
		// if err != nil {
		// 	return err
		// }
		// if err := json.Unmarshal(raw2, &vaultFile); err != nil {
		// 	return err
		// }
		// // Store recovery key so the caller can show it to the user
		// s.migrationRecoveryKey = recoveryKey
	}

	// Unwrap DEK using KEK (password-derived)
	kek := crypto.DeriveKey(password, salt)
	dek, err := crypto.DecryptWithKey(vaultFile.DEKEnc, kek)
	if err != nil {
		return fmt.Errorf("failed to unwrap DEK: %w", err)
	}

	// Verify HMAC integrity (protects all critical fields)
	if !verifyIntegrity(&vaultFile, dek) {
		return ErrTampered
	}

	// Verify signature (tamper detection)
	pubKey, err := signature.ImportPublicKey(vaultFile.PublicKeyPEM)
	if err != nil {
		return err
	}

	if !signature.Verify([]byte(vaultFile.EncryptedData), vaultFile.Signature, pubKey) {
		return ErrTampered
	}

	// Decrypt data with DEK
	dataJSON, err := crypto.DecryptWithKey(vaultFile.EncryptedData, dek)
	if err != nil {
		return err
	}

	var data VaultData
	if err := json.Unmarshal(dataJSON, &data); err != nil {
		return err
	}

	// Decrypt private key with DEK
	privPEM, err := crypto.DecryptWithKey(vaultFile.PrivateKeyEnc, dek)
	if err != nil {
		return err
	}

	privKey, err := signature.ImportPrivateKey(string(privPEM))
	if err != nil {
		return err
	}

	s.filePath = path
	s.password = password
	s.dek = dek
	s.data = &data
	s.file = &vaultFile
	s.keyPair = &signature.KeyPair{
		PrivateKey: privKey,
		PublicKey:  pubKey,
	}

	return nil
}

// Lock locks the vault, clearing all sensitive data from memory
func (s *Store) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.password = ""
	s.dek = nil
	s.data = nil
	s.keyPair = nil
	s.readOnly = false
}

// UnlockReadOnly opens the vault bypassing signature verification.
// All write operations will be refused while in this mode.
func (s *Store) UnlockReadOnly(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := GetVaultPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var vaultFile VaultFile
	if err := json.Unmarshal(raw, &vaultFile); err != nil {
		return err
	}

	// Verify password first — we still refuse wrong passwords
	salt := decodeBytes(vaultFile.PasswordSalt)
	passHash := crypto.HashPassword(password, salt)
	if passHash != vaultFile.PasswordHash {
		return ErrWrongPassword
	}

	// ── Auto-migrate v1 → v2 (envelope encryption) ──
	if vaultFile.DEKEnc == "" {
		recoveryKey, err := migrateV1toV2(&vaultFile, password)
		if err != nil {
			return fmt.Errorf("vault migration failed: %w", err)
		}
		// Re-read the migrated file
		raw2, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw2, &vaultFile); err != nil {
			return err
		}
		s.migrationRecoveryKey = recoveryKey
	}

	// Unwrap DEK
	kek := crypto.DeriveKey(password, salt)
	dek, err := crypto.DecryptWithKey(vaultFile.DEKEnc, kek)
	if err != nil {
		return fmt.Errorf("failed to unwrap DEK: %w", err)
	}

	// Import public key (may still work even if data was tampered)
	pubKey, err := signature.ImportPublicKey(vaultFile.PublicKeyPEM)
	if err != nil {
		return err
	}

	// Decrypt data with DEK — skip signature check intentionally
	dataJSON, err := crypto.DecryptWithKey(vaultFile.EncryptedData, dek)
	if err != nil {
		return err
	}

	var data VaultData
	if err := json.Unmarshal(dataJSON, &data); err != nil {
		return err
	}

	// Decrypt private key with DEK
	privPEM, err := crypto.DecryptWithKey(vaultFile.PrivateKeyEnc, dek)
	if err != nil {
		return err
	}
	privKey, err := signature.ImportPrivateKey(string(privPEM))
	if err != nil {
		return err
	}

	s.filePath = path
	s.password = password
	s.dek = dek
	s.data = &data
	s.file = &vaultFile
	s.keyPair = &signature.KeyPair{PrivateKey: privKey, PublicKey: pubKey}
	s.readOnly = true

	return nil
}

// IsReadOnly returns true when the vault is open in read-only (tamper bypass) mode.
func (s *Store) IsReadOnly() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readOnly
}

// GetNotes returns all notes
func (s *Store) GetNotes() ([]Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data == nil {
		return nil, ErrLocked
	}
	return s.data.Notes, nil
}

// GetNote returns a note by ID
func (s *Store) GetNote(id string) (*Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data == nil {
		return nil, ErrLocked
	}

	for _, note := range s.data.Notes {
		if note.ID == id {
			return &note, nil
		}
	}
	return nil, ErrNotFound
}

// AddNote adds a new note
func (s *Store) AddNote(title, content string, tags []string) (*Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		return nil, ErrLocked
	}
	if s.readOnly {
		return nil, ErrReadOnly
	}

	now := time.Now()
	note := Note{
		ID:        uuid.New().String(),
		Title:     title,
		Content:   content,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.data.Notes = append(s.data.Notes, note)

	if err := s.saveData(); err != nil {
		// Rollback
		s.data.Notes = s.data.Notes[:len(s.data.Notes)-1]
		return nil, err
	}

	return &note, nil
}

// UpdateNote updates an existing note
func (s *Store) UpdateNote(id, title, content string, tags []string) (*Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		return nil, ErrLocked
	}
	if s.readOnly {
		return nil, ErrReadOnly
	}

	for i, note := range s.data.Notes {
		if note.ID == id {
			s.data.Notes[i].Title = title
			s.data.Notes[i].Content = content
			s.data.Notes[i].Tags = tags
			s.data.Notes[i].UpdatedAt = time.Now()

			if err := s.saveData(); err != nil {
				return nil, err
			}

			return &s.data.Notes[i], nil
		}
	}

	return nil, ErrNotFound
}

// DeleteNote deletes a note by ID
func (s *Store) DeleteNote(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		return ErrLocked
	}
	if s.readOnly {
		return ErrReadOnly
	}

	for i, note := range s.data.Notes {
		if note.ID == id {
			s.data.Notes = append(s.data.Notes[:i], s.data.Notes[i+1:]...)
			return s.saveData()
		}
	}

	return ErrNotFound
}

// saveData encrypts and saves data to disk (must be called with lock held)
func (s *Store) saveData() error {
	dataJSON, err := json.Marshal(s.data)
	if err != nil {
		return err
	}

	encData, err := crypto.EncryptWithKey(dataJSON, s.dek)
	if err != nil {
		return err
	}

	sig, err := signature.Sign([]byte(encData), s.keyPair.PrivateKey)
	if err != nil {
		return err
	}

	s.file.EncryptedData = encData
	s.file.Signature = sig

	return s.saveFile()
}

// saveFile writes the vault file to disk (must be called with lock held)
// integrityFields returns the ordered list of critical fields for HMAC computation.
func integrityFields(vf *VaultFile) []string {
	return []string{
		vf.DEKEnc,
		vf.PrivateKeyEnc,
		vf.EncryptedData,
		vf.PublicKeyPEM,
		vf.TOTPSecret,
		vf.RecoveryDEKEnc,
		vf.Signature,
	}
}

// computeIntegrity calculates the HMAC-SHA256 integrity tag for a vault file.
func computeIntegrity(vf *VaultFile, dek []byte) string {
	return crypto.ComputeHMAC(dek, integrityFields(vf)...)
}

// verifyIntegrity checks the vault file's integrity tag.
// Returns true if the tag matches, or if no tag is present (pre-v2.1 vaults).
func verifyIntegrity(vf *VaultFile, dek []byte) bool {
	if vf.Integrity == "" {
		return true // backwards-compatible: no tag means not yet computed
	}
	return crypto.VerifyHMAC(dek, vf.Integrity, integrityFields(vf)...)
}

func (s *Store) saveFile() error {
	// Auto-compute integrity HMAC before every write (if DEK is available)
	if len(s.dek) > 0 {
		s.file.Integrity = computeIntegrity(s.file, s.dek)
	}

	data, err := json.MarshalIndent(s.file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0600)
}

// IsUnlocked checks if vault is unlocked
func (s *Store) IsUnlocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data != nil
}

// GetFile returns the vault file data
func (s *Store) GetFile() *VaultFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.file
}

// SetFile sets the vault file (used internally)
func (s *Store) SetFile(file *VaultFile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file = file
}

// GetPassword returns the current password (used internally)
func (s *Store) GetPassword() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.password
}

// GetDEK returns the Data Encryption Key (used internally for stego/backup)
func (s *Store) GetDEK() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dek
}

// SetPassword updates the in-memory password (used after ChangePassword)
func (s *Store) SetPassword(password string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.password = password
}

// SaveFile persists the current vault file to disk
func (s *Store) SaveFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveFile()
}

// GetMigrationRecoveryKey returns the recovery key generated during v1→v2 migration.
// Returns empty string if no migration occurred. Calling this clears the value.
func (s *Store) GetMigrationRecoveryKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.migrationRecoveryKey
	s.migrationRecoveryKey = ""
	return k
}

// GetKeyPair returns the ECDSA key pair
func (s *Store) GetKeyPair() *signature.KeyPair {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keyPair
}

// GetFilesDir returns the directory for encrypted files
func GetFilesDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".secretvault", "files")
	os.MkdirAll(dir, 0700)
	return dir
}

// ResolveEncPath tries to locate the encrypted file for the given ID.
//  1. Try the canonical path: <filesDir>/<id>.enc
//  2. If not found, scan all .enc files in the directory and read their
//     internal header ID. Return the first match.
//
// Returns ("", ErrNotFound) if no file matches.
func ResolveEncPath(id string) (string, error) {
	return resolveEncPath(id)
}

// resolveEncPath is the internal implementation.
func resolveEncPath(id string) (string, error) {
	// Step 1: try canonical name
	canonical := filepath.Join(GetFilesDir(), id+".enc")
	if _, err := os.Stat(canonical); err == nil {
		return canonical, nil
	}

	// Step 2: quick-scan directory headers
	dir := GetFilesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("cannot read files directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".enc") {
			continue
		}
		p := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		headerID, err := crypto.ReadHeaderID(data)
		if err != nil {
			continue // old-format file or corrupted — skip
		}
		if headerID == id {
			return p, nil
		}
	}

	return "", ErrNotFound
}

// readAndDecryptEncFile reads an .enc file (with or without header) and
// decrypts it. Returns (plaintext, error). Handles both old format (plain
// base64) and new format (header + base64).
func readAndDecryptEncFile(path string, dek []byte) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decryptEncData(data, dek)
}

// decryptEncData decrypts file bytes that may have a header (new format)
// or be plain base64 (legacy format).
func decryptEncData(data, dek []byte) ([]byte, error) {
	if crypto.HasHeader(data) {
		plaintext, _, err := crypto.DecryptFileWithHeader(data, dek)
		return plaintext, err
	}
	// Legacy format: entire content is base64-encoded ciphertext
	return crypto.DecryptWithKey(string(data), dek)
}

// GetFiles returns all file metadata
func (s *Store) GetFiles() ([]FileMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil, ErrLocked
	}
	return s.data.Files, nil
}

// AddFile encrypts and stores a file, returning its metadata
func (s *Store) AddFile(filePath string) (*FileMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		return nil, ErrLocked
	}
	if s.readOnly {
		return nil, ErrReadOnly
	}

	// Read the file
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	// Compute SHA-256 hash of original content
	sum := sha256.Sum256(raw)
	contentHash := hex.EncodeToString(sum[:])

	// Detect MIME type by extension
	mimeType := mimeFromExt(strings.ToLower(filepath.Ext(filePath)))

	now := time.Now()
	meta := FileMetadata{
		ID:           uuid.New().String(),
		OriginalName: filepath.Base(filePath),
		MimeType:     mimeType,
		Size:         info.Size(),
		ContentHash:  contentHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Sign the metadata with ECDSA
	metaSig, err := signMetadata(&meta, s.keyPair)
	if err != nil {
		return nil, err
	}
	meta.Signature = metaSig

	// Encrypt the file with internal ID header
	encryptedBytes, err := crypto.EncryptFileWithHeader(raw, s.dek, meta.ID)
	if err != nil {
		return nil, err
	}

	// Save encrypted file to disk
	encPath := filepath.Join(GetFilesDir(), meta.ID+".enc")
	if err := os.WriteFile(encPath, encryptedBytes, 0600); err != nil {
		return nil, err
	}

	s.data.Files = append(s.data.Files, meta)
	if err := s.saveData(); err != nil {
		s.data.Files = s.data.Files[:len(s.data.Files)-1]
		os.Remove(encPath)
		return nil, err
	}

	return &meta, nil
}

// TamperDetail describes a single metadata field that failed verification
type TamperDetail struct {
	Field    string `json:"field"`
	Reason   string `json:"reason"`
	Stored   string `json:"stored"`
	Expected string `json:"expected"`
}

// GetFileTamperDetails returns per-field tamper analysis for a file
func (s *Store) GetFileTamperDetails(id string) ([]TamperDetail, *FileMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil, nil, ErrLocked
	}

	var meta *FileMetadata
	for i := range s.data.Files {
		if s.data.Files[i].ID == id {
			meta = &s.data.Files[i]
			break
		}
	}
	if meta == nil {
		return nil, nil, ErrNotFound
	}

	var details []TamperDetail

	// Check ECDSA signature — if valid, nothing to report
	if verifyMetadata(meta, s.keyPair) {
		// Signature is fine; check content hash against actual encrypted file
		encPath, err := resolveEncPath(id)
		if err != nil {
			details = append(details, TamperDetail{
				Field:  "encrypted_file",
				Reason: "Encrypted file is missing from disk (checked by name and internal ID scan)",
				Stored: filepath.Join(GetFilesDir(), id+".enc"),
			})
			return details, meta, nil
		}
		decrypted, err := readAndDecryptEncFile(encPath, s.dek)
		if err != nil {
			details = append(details, TamperDetail{
				Field:  "encrypted_content",
				Reason: "AES-256-GCM authentication failed — encrypted bytes on disk have been modified or corrupted",
				Stored: encPath,
			})
			return details, meta, nil
		}
		sum := sha256.Sum256(decrypted)
		actualHash := hex.EncodeToString(sum[:])
		if actualHash != meta.ContentHash {
			details = append(details, TamperDetail{
				Field:    "content_hash",
				Reason:   "File content was modified after being stored",
				Stored:   meta.ContentHash[:16] + "…",
				Expected: actualHash[:16] + "…",
			})
		}
		return details, meta, nil
	}

	// Signature failed — reconstruct what the signed payload should look like
	// and compare against stored metadata fields
	details = append(details, TamperDetail{
		Field:  "ecdsa_signature",
		Reason: "ECDSA P-256 signature does not match metadata fields",
		Stored: meta.Signature[:20] + "…",
	})

	// Try to narrow down which field changed by probing canonical string components
	canonical := meta.ID + "|" + meta.OriginalName + "|" + meta.MimeType + "|" +
		hex.EncodeToString([]byte(meta.ContentHash)) + "|" +
		meta.CreatedAt.UTC().Format(time.RFC3339)

	// Report the fields that are part of the signed payload so user knows what is suspect
	details = append(details, TamperDetail{
		Field:  "original_name",
		Reason: "Included in signed payload — modification would break signature",
		Stored: meta.OriginalName,
	})
	details = append(details, TamperDetail{
		Field:  "created_at",
		Reason: "Included in signed payload — modification would break signature",
		Stored: meta.CreatedAt.UTC().Format(time.RFC3339),
	})
	details = append(details, TamperDetail{
		Field:  "content_hash",
		Reason: "Included in signed payload — modification would break signature",
		Stored: meta.ContentHash[:32] + "…",
	})
	_ = canonical

	return details, meta, nil
}

// DecryptFileForced decrypts a file bypassing ECDSA signature verification.
// Used for tamper-bypass recovery. Returns raw bytes even if metadata is suspect.
func (s *Store) DecryptFileForced(id string) ([]byte, *FileMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil, nil, ErrLocked
	}

	var meta *FileMetadata
	for i := range s.data.Files {
		if s.data.Files[i].ID == id {
			meta = &s.data.Files[i]
			break
		}
	}
	if meta == nil {
		return nil, nil, ErrNotFound
	}

	encPath, err := resolveEncPath(id)
	if err != nil {
		return nil, meta, fmt.Errorf("encrypted file missing from disk (checked by name and internal ID scan): %w", err)
	}

	encData, err := os.ReadFile(encPath)
	if err != nil {
		return nil, meta, fmt.Errorf("cannot read encrypted file: %w", err)
	}

	cp := *meta
	cp.Tampered = true

	decrypted, decErr := decryptEncData(encData, s.dek)
	if decErr != nil {
		// AES-GCM decryption failed — the encrypted content has been corrupted.
		// Return the raw encrypted bytes so the user can at least save them.
		return encData, &cp, ErrDecryptFailed
	}

	return decrypted, &cp, nil
}

// DecryptFile decrypts and returns file bytes, verifying ECDSA signature first
func (s *Store) DecryptFile(id string) ([]byte, *FileMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil, nil, ErrLocked
	}

	var meta *FileMetadata
	for i := range s.data.Files {
		if s.data.Files[i].ID == id {
			meta = &s.data.Files[i]
			break
		}
	}
	if meta == nil {
		return nil, nil, ErrNotFound
	}

	// Verify ECDSA signature on metadata
	if !verifyMetadata(meta, s.keyPair) {
		cp := *meta
		cp.Tampered = true
		return nil, &cp, ErrTampered
	}

	encPath, err := resolveEncPath(id)
	if err != nil {
		cp := *meta
		cp.Tampered = true
		return nil, &cp, ErrTampered
	}

	decrypted, err := readAndDecryptEncFile(encPath, s.dek)
	if err != nil {
		// AES-GCM authentication failed → encrypted content was modified on disk
		cp := *meta
		cp.Tampered = true
		return nil, &cp, ErrTampered
	}

	// Verify content hash
	sum := sha256.Sum256(decrypted)
	if hex.EncodeToString(sum[:]) != meta.ContentHash {
		cp := *meta
		cp.Tampered = true
		return nil, &cp, ErrTampered
	}

	return decrypted, meta, nil
}

// DeleteFile removes an encrypted file and its metadata
func (s *Store) DeleteFile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		return ErrLocked
	}
	if s.readOnly {
		return ErrReadOnly
	}

	for i, f := range s.data.Files {
		if f.ID == id {
			s.data.Files = append(s.data.Files[:i], s.data.Files[i+1:]...)
			// Remove encrypted file — try resolveEncPath to handle renamed files
			if encPath, err := resolveEncPath(id); err == nil {
				os.Remove(encPath)
			} else {
				os.Remove(filepath.Join(GetFilesDir(), id+".enc"))
			}
			return s.saveData()
		}
	}
	return ErrNotFound
}

// ImportFile restores a file from backup: writes the encrypted data to disk
// and adds the metadata entry. The caller is responsible for passing valid
// encrypted content that was produced with the same DEK.
func (s *Store) ImportFile(meta FileMetadata, encryptedData string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		return ErrLocked
	}
	if s.readOnly {
		return ErrReadOnly
	}

	// Check for duplicate by content hash
	for _, f := range s.data.Files {
		if f.ContentHash == meta.ContentHash {
			return nil // already exists, skip silently
		}
	}

	// Write encrypted file to disk with internal ID header
	fileBytes := crypto.PrependHeaderToBase64([]byte(encryptedData), meta.ID)
	encPath := filepath.Join(GetFilesDir(), meta.ID+".enc")
	if err := os.WriteFile(encPath, fileBytes, 0600); err != nil {
		return err
	}

	s.data.Files = append(s.data.Files, meta)
	if err := s.saveData(); err != nil {
		s.data.Files = s.data.Files[:len(s.data.Files)-1]
		os.Remove(encPath)
		return err
	}
	return nil
}

// ImportNote restores a note from backup, preserving its original ID and timestamps.
// Returns false if a note with the same title and content already exists (duplicate).
func (s *Store) ImportNote(note Note) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		return false, ErrLocked
	}
	if s.readOnly {
		return false, ErrReadOnly
	}

	// Deduplicate by title + content
	for _, n := range s.data.Notes {
		if n.Title == note.Title && n.Content == note.Content {
			return false, nil
		}
	}

	s.data.Notes = append(s.data.Notes, note)
	if err := s.saveData(); err != nil {
		s.data.Notes = s.data.Notes[:len(s.data.Notes)-1]
		return false, err
	}
	return true, nil
}

// SearchNotes searches notes by title (case-insensitive)
func (s *Store) SearchNotes(query string) ([]Note, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		return nil, ErrLocked
	}

	q := strings.ToLower(query)
	var results []Note
	for _, n := range s.data.Notes {
		if strings.Contains(strings.ToLower(n.Title), q) {
			results = append(results, n)
		}
	}
	return results, nil
}

// SignFileMetadata builds a canonical string from metadata fields and signs it.
// Exported so backup import can re-sign file metadata with the current vault's key.
func SignFileMetadata(m *FileMetadata, kp *signature.KeyPair) (string, error) {
	payload := m.ID + "|" + m.OriginalName + "|" + m.MimeType + "|" +
		hex.EncodeToString([]byte(m.ContentHash)) + "|" +
		m.CreatedAt.UTC().Format(time.RFC3339)
	return signature.Sign([]byte(payload), kp.PrivateKey)
}

// signMetadata builds a canonical string from metadata fields and signs it
func signMetadata(m *FileMetadata, kp *signature.KeyPair) (string, error) {
	return SignFileMetadata(m, kp)
}

// verifyMetadata verifies the ECDSA signature on metadata
func verifyMetadata(m *FileMetadata, kp *signature.KeyPair) bool {
	payload := m.ID + "|" + m.OriginalName + "|" + m.MimeType + "|" +
		hex.EncodeToString([]byte(m.ContentHash)) + "|" +
		m.CreatedAt.UTC().Format(time.RFC3339)
	return signature.Verify([]byte(payload), m.Signature, kp.PublicKey)
}

// mimeFromExt returns a MIME type from file extension
func mimeFromExt(ext string) string {
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
