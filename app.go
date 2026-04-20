package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"secretvault/internal/crypto"
	"secretvault/internal/signature"
	"secretvault/internal/stego"
	"secretvault/internal/store"
	"secretvault/internal/totp"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx   context.Context
	store *store.Store
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		store: store.NewStore(),
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// --- Response types ---

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type NoteResponse struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func noteToResponse(n *store.Note) NoteResponse {
	return NoteResponse{
		ID:        n.ID,
		Title:     n.Title,
		Content:   n.Content,
		Tags:      n.Tags,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
		UpdatedAt: n.UpdatedAt.Format(time.RFC3339),
	}
}

func successResp(data interface{}) Response {
	return Response{Success: true, Data: data}
}

func errorResp(err string) Response {
	return Response{Success: false, Error: err}
}

// --- Auth Methods ---

// CheckVaultExists checks if vault file exists
func (a *App) CheckVaultExists() Response {
	return successResp(a.store.VaultExists())
}

// CreateVault creates a new vault with password
func (a *App) CreateVault(password string) Response {
	if len(password) < 8 {
		return errorResp("Password must be at least 8 characters")
	}
	recoveryKey, err := a.store.CreateVault(password)
	if err != nil {
		return errorResp(err.Error())
	}
	return successResp(map[string]string{
		"recovery_key": recoveryKey,
	})
}

// UnlockVault unlocks the vault with password (and optional TOTP)
func (a *App) UnlockVault(password string, totpCode string) Response {
	// Check lockout first
	if locked, remaining := store.IsLockedOut(); locked {
		hours := int(remaining.Hours())
		minutes := int(remaining.Minutes()) % 60
		return Response{
			Success: false,
			Error:   "TOTP_LOCKED",
			Data: map[string]interface{}{
				"remaining_seconds": int(remaining.Seconds()),
				"message":           fmt.Sprintf("App is locked due to too many failed TOTP attempts. Try again in %dh %dm.", hours, minutes),
			},
		}
	}

	// First try to read the vault to check TOTP
	if a.store.VaultExists() {
		raw, err := os.ReadFile(store.GetVaultPath())
		if err != nil {
			return errorResp(err.Error())
		}
		var vaultFile store.VaultFile
		if err := json.Unmarshal(raw, &vaultFile); err != nil {
			return errorResp(err.Error())
		}

		// Check if TOTP is enabled
		if vaultFile.TOTPEnabled && vaultFile.TOTPSecret != "" {
			var totpSecret []byte
			var totpDecrypted bool

			if vaultFile.DEKEnc != "" {
				// v2 vault: derive KEK to unwrap DEK, then decrypt TOTP secret
				salt, _ := base64.StdEncoding.DecodeString(vaultFile.PasswordSalt)
				kek := crypto.DeriveKey(password, salt)
				dek, dekErr := crypto.DecryptWithKey(vaultFile.DEKEnc, kek)
				if dekErr == nil {
					ts, err := crypto.DecryptWithKey(vaultFile.TOTPSecret, dek)
					if err == nil {
						totpSecret = ts
						totpDecrypted = true
					}
				}
			} else {
				// v1 vault: TOTP was encrypted with password-based crypto.Encrypt
				ts, err := crypto.Decrypt(vaultFile.TOTPSecret, password)
				if err == nil {
					totpSecret = ts
					totpDecrypted = true
				}
			}

			if totpDecrypted {
				if totpCode == "" {
					remaining := store.RemainingAttempts()
					return Response{
						Success: false,
						Error:   "TOTP_REQUIRED",
						Data: map[string]interface{}{
							"remaining_attempts": remaining,
						},
					}
				}
				if !totp.ValidateCode(totpCode, string(totpSecret)) {
					state := store.RecordTOTPFailure()
					remaining := store.MaxTOTPAttempts - state.TOTPFailures
					if remaining < 0 {
						remaining = 0
					}
					if !state.LockedUntil.IsZero() {
						rem := time.Until(state.LockedUntil)
						hours := int(rem.Hours())
						minutes := int(rem.Minutes()) % 60
						return Response{
							Success: false,
							Error:   "TOTP_LOCKED",
							Data: map[string]interface{}{
								"remaining_seconds": int(rem.Seconds()),
								"message":           fmt.Sprintf("Too many failed attempts. App is locked for %dh %dm.", hours, minutes),
							},
						}
					}
					return Response{
						Success: false,
						Error:   "TOTP_INVALID",
						Data: map[string]interface{}{
							"remaining_attempts": remaining,
						},
					}
				}
			}
		}
	}

	if err := a.store.Unlock(password); err != nil {
		if err == store.ErrTampered {
			return Response{
				Success: false,
				Error:   "VAULT_TAMPERED",
				Data: map[string]string{
					"detail": "The vault's ECDSA signature does not match its contents. " +
						"The encrypted data may have been modified outside the application.",
				},
			}
		}
		return errorResp(err.Error())
	}

	// Successful unlock — reset TOTP failure counter
	store.ResetTOTPFailures()

	// Check if vault was migrated from v1 → v2
	if migrationKey := a.store.GetMigrationRecoveryKey(); migrationKey != "" {
		return successResp(map[string]interface{}{
			"migrated":     true,
			"recovery_key": migrationKey,
		})
	}

	return successResp(true)
}

// UnlockVaultReadOnly opens the vault in read-only mode, bypassing tamper detection.
// Use this when the user consciously chooses to open a tampered vault to recover data.
func (a *App) UnlockVaultReadOnly(password string) Response {
	if err := a.store.UnlockReadOnly(password); err != nil {
		return errorResp(err.Error())
	}
	// Check if vault was migrated from v1 → v2
	if migrationKey := a.store.GetMigrationRecoveryKey(); migrationKey != "" {
		return successResp(map[string]interface{}{
			"migrated":     true,
			"recovery_key": migrationKey,
		})
	}
	return successResp(true)
}

// IsReadOnly returns whether the vault is currently open in read-only mode.
func (a *App) IsReadOnly() Response {
	return successResp(a.store.IsReadOnly())
}

// LockVault locks the vault
func (a *App) LockVault() Response {
	a.store.Lock()
	return successResp(true)
}

// IsUnlocked checks if vault is unlocked
func (a *App) IsUnlocked() Response {
	return successResp(a.store.IsUnlocked())
}

// --- TOTP Methods ---

// SetupTOTP generates a new TOTP secret
func (a *App) SetupTOTP() Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	secret, qrBase64, err := totp.GenerateSecret("user@secretvault")
	if err != nil {
		return errorResp(err.Error())
	}

	return successResp(map[string]string{
		"secret": secret,
		"qr":     qrBase64,
	})
}

// EnableTOTP enables TOTP with the given secret and verification code
func (a *App) EnableTOTP(secret string, code string) Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	// Validate code
	if !totp.ValidateCode(code, secret) {
		return errorResp("Invalid verification code")
	}

	// Encrypt and save TOTP secret
	encSecret, err := crypto.EncryptWithKey([]byte(secret), a.store.GetDEK())
	if err != nil {
		return errorResp(err.Error())
	}

	file := a.store.GetFile()
	file.TOTPSecret = encSecret
	file.TOTPEnabled = true
	a.store.SetFile(file)

	if err := a.store.SaveFile(); err != nil {
		return errorResp(err.Error())
	}

	return successResp(true)
}

// DisableTOTP disables TOTP
func (a *App) DisableTOTP() Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	file := a.store.GetFile()
	file.TOTPSecret = ""
	file.TOTPEnabled = false
	a.store.SetFile(file)

	if err := a.store.SaveFile(); err != nil {
		return errorResp(err.Error())
	}

	return successResp(true)
}

// IsTOTPEnabled checks if TOTP is enabled
func (a *App) IsTOTPEnabled() Response {
	if a.store.GetFile() == nil {
		// Read from file directly
		raw, err := os.ReadFile(store.GetVaultPath())
		if err != nil {
			return successResp(false)
		}
		var vaultFile store.VaultFile
		if err := json.Unmarshal(raw, &vaultFile); err != nil {
			return successResp(false)
		}
		return successResp(vaultFile.TOTPEnabled)
	}
	return successResp(a.store.GetFile().TOTPEnabled)
}

// --- Notes Methods ---

// GetNotes returns all notes
func (a *App) GetNotes() Response {
	notes, err := a.store.GetNotes()
	if err != nil {
		return errorResp(err.Error())
	}

	result := make([]NoteResponse, len(notes))
	for i, n := range notes {
		nn := n
		result[i] = noteToResponse(&nn)
	}

	return successResp(result)
}

// GetNote returns a single note by ID
func (a *App) GetNote(id string) Response {
	note, err := a.store.GetNote(id)
	if err != nil {
		return errorResp(err.Error())
	}
	return successResp(noteToResponse(note))
}

// AddNote creates a new note
func (a *App) AddNote(title, content string, tags []string) Response {
	if tags == nil {
		tags = []string{}
	}
	note, err := a.store.AddNote(title, content, tags)
	if err != nil {
		return errorResp(err.Error())
	}
	return successResp(noteToResponse(note))
}

// UpdateNote updates an existing note
func (a *App) UpdateNote(id, title, content string, tags []string) Response {
	if tags == nil {
		tags = []string{}
	}
	note, err := a.store.UpdateNote(id, title, content, tags)
	if err != nil {
		return errorResp(err.Error())
	}
	return successResp(noteToResponse(note))
}

// DeleteNote deletes a note
func (a *App) DeleteNote(id string) Response {
	if err := a.store.DeleteNote(id); err != nil {
		return errorResp(err.Error())
	}
	return successResp(true)
}

// --- Steganography Methods ---

// HideInImage hides a note's content in the provided cover image and opens a save dialog
func (a *App) HideInImage(noteID string, imagePath string) Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	if imagePath == "" {
		return errorResp("No image selected")
	}

	note, err := a.store.GetNote(noteID)
	if err != nil {
		return errorResp(err.Error())
	}

	// Encrypt note content before hiding
	encrypted, err := crypto.EncryptWithKey([]byte(note.Content), a.store.GetDEK())
	if err != nil {
		return errorResp(err.Error())
	}

	// Hide encrypted data in image
	result, err := stego.HideData(imagePath, []byte(encrypted))
	if err != nil {
		return errorResp(err.Error())
	}

	// Save dialog
	savePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Save Steganographic Image",
		DefaultFilename: "secret_image.png",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "PNG Images", Pattern: "*.png"},
		},
	})
	if err != nil {
		return errorResp(err.Error())
	}
	if savePath == "" {
		return errorResp("No save location selected")
	}

	if err := os.WriteFile(savePath, result, 0644); err != nil {
		return errorResp(err.Error())
	}

	resultBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(result)
	return successResp(map[string]interface{}{
		"path":    savePath,
		"size":    fmt.Sprintf("%.2f KB", float64(len(result))/1024),
		"preview": resultBase64,
	})
}

// ExtractFromImage extracts hidden data from an image
func (a *App) ExtractFromImage() Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	// Open file dialog
	imagePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Steganographic Image",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "PNG Images", Pattern: "*.png"},
		},
	})
	if err != nil {
		return errorResp(err.Error())
	}
	if imagePath == "" {
		return errorResp("No image selected")
	}

	// Extract data
	data, err := stego.ExtractData(imagePath)
	if err != nil {
		return errorResp(err.Error())
	}

	// Decrypt the extracted data
	decrypted, err := crypto.DecryptWithKey(string(data), a.store.GetDEK())
	if err != nil {
		return errorResp("Could not decrypt extracted data. The image may have been created with a different vault or an older version.")
	}

	return successResp(map[string]string{
		"content": string(decrypted),
	})
}

// --- Backup Methods ---

// backupFileEntry holds one file's plaintext content + metadata for the backup bundle.
type backupFileEntry struct {
	Meta          store.FileMetadata `json:"meta"`
	EncryptedData string             `json:"encrypted_data"` // AES-256-GCM ciphertext (encrypted with backup key)
}

// ExportBackup creates a portable encrypted backup of the entire vault.
// The user must provide the Recovery Key. A fresh backup key is derived from it
// (PBKDF2 with a random salt) so the .svault file can be imported into ANY vault
// as long as the Recovery Key is known.
//
// Payload: notes (plaintext JSON) + files (re-encrypted with backup key) + settings.
func (a *App) ExportBackup(recoveryKey string) Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	// Validate recovery key against stored hash
	if err := a.store.ValidateRecoveryKey(recoveryKey); err != nil {
		return errorResp(err.Error())
	}

	// --- Derive a backup encryption key from the recovery key ---
	backupSalt, err := crypto.GenerateSalt()
	if err != nil {
		return errorResp(err.Error())
	}
	backupKey := crypto.DeriveKey(store.NormalizeRecoveryKey(recoveryKey), backupSalt)

	// --- Gather data ---
	notes, err := a.store.GetNotes()
	if err != nil {
		return errorResp(err.Error())
	}

	files, err := a.store.GetFiles()
	if err != nil {
		return errorResp(err.Error())
	}

	dek := a.store.GetDEK()

	// Decrypt each file with the vault's DEK, then re-encrypt with the backup key
	var fileEntries []backupFileEntry
	for _, f := range files {
		encPath := filepath.Join(store.GetFilesDir(), f.ID+".enc")
		encBytes, err := os.ReadFile(encPath)
		if err != nil {
			// Canonical name not found — try resolving by internal header ID
			encPath, err = store.ResolveEncPath(f.ID)
			if err != nil {
				continue // skip files missing on disk
			}
			encBytes, err = os.ReadFile(encPath)
			if err != nil {
				continue
			}
		}

		// Decrypt with current DEK (handles both header and legacy format)
		var plaintext []byte
		if crypto.HasHeader(encBytes) {
			plaintext, _, err = crypto.DecryptFileWithHeader(encBytes, dek)
		} else {
			plaintext, err = crypto.DecryptWithKey(string(encBytes), dek)
		}
		if err != nil {
			continue // skip corrupted files
		}

		// Re-encrypt with backup key
		reEncrypted, err := crypto.EncryptWithKey(plaintext, backupKey)
		if err != nil {
			continue
		}

		fileEntries = append(fileEntries, backupFileEntry{
			Meta:          f,
			EncryptedData: reEncrypted,
		})
	}

	vf := a.store.GetFile()

	// --- Build backup payload ---
	backup := map[string]interface{}{
		"version":      3,
		"exported_at":  time.Now().Format(time.RFC3339),
		"notes":        notes,
		"files":        fileEntries,
		"totp_enabled": vf.TOTPEnabled,
	}

	backupJSON, err := json.Marshal(backup)
	if err != nil {
		return errorResp(err.Error())
	}

	// Encrypt the whole payload with the backup key
	encrypted, err := crypto.EncryptWithKey(backupJSON, backupKey)
	if err != nil {
		return errorResp(err.Error())
	}

	// Sign with the vault's ECDSA key
	kp := a.store.GetKeyPair()
	sig, err := signature.Sign([]byte(encrypted), kp.PrivateKey)
	if err != nil {
		return errorResp(err.Error())
	}

	pubPEM, err := signature.ExportPublicKey(kp.PublicKey)
	if err != nil {
		return errorResp(err.Error())
	}

	// --- Outer wrapper (unencrypted metadata) ---
	backupFile := map[string]interface{}{
		"version":        3,
		"backup_salt":    base64.StdEncoding.EncodeToString(backupSalt),
		"encrypted_data": encrypted,
		"signature":      sig,
		"public_key":     pubPEM,
	}

	backupFileJSON, err := json.MarshalIndent(backupFile, "", "  ")
	if err != nil {
		return errorResp(err.Error())
	}

	// --- Save dialog ---
	savePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Export Encrypted Backup",
		DefaultFilename: fmt.Sprintf("secretvault_backup_%s.svault", time.Now().Format("20060102_150405")),
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Secret Vault Backup", Pattern: "*.svault"},
		},
	})
	if err != nil {
		return errorResp(err.Error())
	}
	if savePath == "" {
		return errorResp("No save location selected")
	}

	if err := os.WriteFile(savePath, backupFileJSON, 0600); err != nil {
		return errorResp(err.Error())
	}

	return successResp(map[string]interface{}{
		"path":  savePath,
		"notes": len(notes),
		"files": len(fileEntries),
	})
}

// ImportBackup restores a full vault backup using the Recovery Key.
// Files are decrypted with the backup key and re-encrypted with the current vault's DEK.
// Duplicates are detected and skipped (notes by title+content, files by content hash).
func (a *App) ImportBackup(recoveryKey string) Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	// --- Open file dialog ---
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Import Encrypted Backup",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Secret Vault Backup", Pattern: "*.svault"},
		},
	})
	if err != nil {
		return errorResp(err.Error())
	}
	if filePath == "" {
		return errorResp("No file selected")
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return errorResp(err.Error())
	}

	var backupFileData map[string]interface{}
	if err := json.Unmarshal(raw, &backupFileData); err != nil {
		return errorResp("Invalid backup file format")
	}

	encData, ok := backupFileData["encrypted_data"].(string)
	if !ok {
		return errorResp("Invalid backup file: missing encrypted_data")
	}

	sig, ok := backupFileData["signature"].(string)
	if !ok {
		return errorResp("Invalid backup file: missing signature")
	}

	pubKeyPEM, ok := backupFileData["public_key"].(string)
	if !ok {
		return errorResp("Invalid backup file: missing public_key")
	}

	// Verify ECDSA signature
	pubKey, err := signature.ImportPublicKey(pubKeyPEM)
	if err != nil {
		return errorResp("Invalid public key in backup")
	}
	if !signature.Verify([]byte(encData), sig, pubKey) {
		return errorResp("Backup signature verification failed — file may be tampered")
	}

	// --- Derive the backup key from the Recovery Key ---
	backupSaltB64, ok := backupFileData["backup_salt"].(string)
	if !ok {
		return errorResp("Invalid backup file: missing backup_salt")
	}
	backupSalt, err := base64.StdEncoding.DecodeString(backupSaltB64)
	if err != nil {
		return errorResp("Invalid backup salt")
	}
	backupKey := crypto.DeriveKey(store.NormalizeRecoveryKey(recoveryKey), backupSalt)

	// Decrypt the payload
	decrypted, err := crypto.DecryptWithKey(encData, backupKey)
	if err != nil {
		return errorResp("Could not decrypt backup. Make sure the Recovery Key is correct.")
	}

	var backup map[string]interface{}
	if err := json.Unmarshal(decrypted, &backup); err != nil {
		return errorResp("Corrupted backup data")
	}

	// --- Import Notes ---
	notesRaw, err := json.Marshal(backup["notes"])
	if err != nil {
		return errorResp("Invalid notes data in backup")
	}

	var notes []store.Note
	if err := json.Unmarshal(notesRaw, &notes); err != nil {
		return errorResp("Invalid notes format in backup")
	}

	importedNotes := 0
	for _, note := range notes {
		added, err := a.store.ImportNote(note)
		if err == nil && added {
			importedNotes++
		}
	}

	// --- Import Files ---
	dek := a.store.GetDEK()
	kp := a.store.GetKeyPair()
	importedFiles := 0

	if filesRaw, ok := backup["files"]; ok {
		filesJSON, err := json.Marshal(filesRaw)
		if err == nil {
			var fileEntries []backupFileEntry
			if json.Unmarshal(filesJSON, &fileEntries) == nil {
				for _, entry := range fileEntries {
					// Decrypt with backup key
					plaintext, err := crypto.DecryptWithKey(entry.EncryptedData, backupKey)
					if err != nil {
						continue
					}

					// Re-encrypt with the current vault's DEK
					reEncrypted, err := crypto.EncryptWithKey(plaintext, dek)
					if err != nil {
						continue
					}

					// Re-sign the metadata with the current vault's ECDSA key
					meta := entry.Meta
					meta.Tampered = false
					metaSig, err := store.SignFileMetadata(&meta, kp)
					if err != nil {
						continue
					}
					meta.Signature = metaSig

					if err := a.store.ImportFile(meta, reEncrypted); err == nil {
						importedFiles++
					}
				}
			}
		}
	}

	return successResp(map[string]interface{}{
		"imported_notes": importedNotes,
		"total_notes":    len(notes),
		"imported_files": importedFiles,
	})
}

// RestoreFromBackup creates a brand-new vault from a .svault backup file.
// This is used on first launch (no vault exists) so the user can restore
// their data using only the Recovery Key + the backup file.
//
// Flow: create new vault with the given password → decrypt backup with
// recovery key → import notes + files → return new recovery key.
func (a *App) RestoreFromBackup(newPassword, recoveryKey string) Response {
	if a.store.VaultExists() {
		return errorResp("A vault already exists. Use Import Backup from Settings instead.")
	}
	if len(newPassword) < 8 {
		return errorResp("Password must be at least 8 characters")
	}
	if recoveryKey == "" {
		return errorResp("Recovery Key is required")
	}

	// --- 1. Open file dialog to pick the .svault file ---
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Backup File",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Secret Vault Backup", Pattern: "*.svault"},
		},
	})
	if err != nil {
		return errorResp(err.Error())
	}
	if filePath == "" {
		return errorResp("No file selected")
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return errorResp(err.Error())
	}

	var backupFileData map[string]interface{}
	if err := json.Unmarshal(raw, &backupFileData); err != nil {
		return errorResp("Invalid backup file format")
	}

	encData, ok := backupFileData["encrypted_data"].(string)
	if !ok {
		return errorResp("Invalid backup file: missing encrypted_data")
	}

	// Signature verification (optional — the public key in the backup may
	// belong to the old vault, but we still verify to detect file corruption)
	if sig, ok := backupFileData["signature"].(string); ok {
		if pubPEM, ok := backupFileData["public_key"].(string); ok {
			pubKey, err := signature.ImportPublicKey(pubPEM)
			if err == nil && !signature.Verify([]byte(encData), sig, pubKey) {
				return errorResp("Backup signature verification failed — file may be corrupted")
			}
		}
	}

	// --- 2. Derive backup key and decrypt ---
	backupSaltB64, ok := backupFileData["backup_salt"].(string)
	if !ok {
		return errorResp("Invalid backup file: missing backup_salt")
	}
	backupSalt, err := base64.StdEncoding.DecodeString(backupSaltB64)
	if err != nil {
		return errorResp("Invalid backup salt")
	}
	backupKey := crypto.DeriveKey(store.NormalizeRecoveryKey(recoveryKey), backupSalt)

	decrypted, err := crypto.DecryptWithKey(encData, backupKey)
	if err != nil {
		return errorResp("Could not decrypt backup. Make sure the Recovery Key is correct.")
	}

	var backup map[string]interface{}
	if err := json.Unmarshal(decrypted, &backup); err != nil {
		return errorResp("Corrupted backup data")
	}

	// --- 3. Create a fresh vault ---
	newRecoveryKey, err := a.store.CreateVault(newPassword)
	if err != nil {
		return errorResp("Failed to create vault: " + err.Error())
	}

	// Vault is now unlocked with a new DEK + new ECDSA key pair.

	// --- 4. Import notes ---
	notesRaw, _ := json.Marshal(backup["notes"])
	var notes []store.Note
	json.Unmarshal(notesRaw, &notes)

	importedNotes := 0
	for _, note := range notes {
		added, err := a.store.ImportNote(note)
		if err == nil && added {
			importedNotes++
		}
	}

	// --- 5. Import files ---
	dek := a.store.GetDEK()
	kp := a.store.GetKeyPair()
	importedFiles := 0

	if filesRaw, ok := backup["files"]; ok {
		filesJSON, _ := json.Marshal(filesRaw)
		var fileEntries []backupFileEntry
		if json.Unmarshal(filesJSON, &fileEntries) == nil {
			for _, entry := range fileEntries {
				plaintext, err := crypto.DecryptWithKey(entry.EncryptedData, backupKey)
				if err != nil {
					continue
				}
				reEncrypted, err := crypto.EncryptWithKey(plaintext, dek)
				if err != nil {
					continue
				}
				meta := entry.Meta
				meta.Tampered = false
				metaSig, err := store.SignFileMetadata(&meta, kp)
				if err != nil {
					continue
				}
				meta.Signature = metaSig
				if err := a.store.ImportFile(meta, reEncrypted); err == nil {
					importedFiles++
				}
			}
		}
	}

	return successResp(map[string]interface{}{
		"recovery_key":   newRecoveryKey,
		"imported_notes": importedNotes,
		"imported_files": importedFiles,
	})
}

// --- Change Password ---

// ChangePassword changes the vault password
func (a *App) ChangePassword(oldPassword, newPassword string) Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	if len(newPassword) < 8 {
		return errorResp("New password must be at least 8 characters")
	}

	// Verify old password
	file := a.store.GetFile()
	oldSalt, _ := base64.StdEncoding.DecodeString(file.PasswordSalt)
	passHash := crypto.HashPassword(oldPassword, oldSalt)
	if passHash != file.PasswordHash {
		return errorResp("Current password is incorrect")
	}

	// Generate new salt and hash for new password
	newSalt, err := crypto.GenerateSalt()
	if err != nil {
		return errorResp(err.Error())
	}
	newPassHash := crypto.HashPassword(newPassword, newSalt)

	// Derive new KEK and re-wrap the existing DEK
	newKEK := crypto.DeriveKey(newPassword, newSalt)
	newDEKEnc, err := crypto.EncryptWithKey(a.store.GetDEK(), newKEK)
	if err != nil {
		return errorResp(err.Error())
	}

	// Re-encrypt private key with DEK is not needed (DEK hasn't changed)
	// Just update the password-related fields in the vault file
	file.PasswordSalt = base64.StdEncoding.EncodeToString(newSalt)
	file.PasswordHash = newPassHash
	file.DEKEnc = newDEKEnc

	// Update in-memory password
	a.store.SetPassword(newPassword)

	// Save updated vault file
	if err := a.store.SaveFile(); err != nil {
		return errorResp(err.Error())
	}

	return successResp(true)
}

// GetVaultInfo returns information about the vault
func (a *App) GetVaultInfo() Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	notes, _ := a.store.GetNotes()
	files, _ := a.store.GetFiles()
	file := a.store.GetFile()

	info, _ := os.Stat(store.GetVaultPath())
	var fileSize string
	if info != nil {
		fileSize = fmt.Sprintf("%.2f KB", float64(info.Size())/1024)
	}

	return successResp(map[string]interface{}{
		"notes_count":  len(notes),
		"files_count":  len(files),
		"totp_enabled": file.TOTPEnabled,
		"file_size":    fileSize,
		"vault_path":   store.GetVaultPath(),
	})
}

// SelectImage opens a file dialog to select an image for steganography
func (a *App) SelectImage() Response {
	imagePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Image",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Images", Pattern: "*.png;*.jpg;*.jpeg"},
		},
	})
	if err != nil {
		return errorResp(err.Error())
	}
	if imagePath == "" {
		return errorResp("No image selected")
	}

	// Get image info
	info, err := os.Stat(imagePath)
	if err != nil {
		return errorResp(err.Error())
	}

	// Read image and encode as base64 for preview
	imgBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return errorResp(err.Error())
	}
	imgBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imgBytes)

	return successResp(map[string]interface{}{
		"path":    imagePath,
		"name":    filepath.Base(imagePath),
		"size":    fmt.Sprintf("%.2f KB", float64(info.Size())/1024),
		"preview": imgBase64,
	})
}

// PreviewHideInImage hides note data in image and returns the result as base64 (no save dialog)
func (a *App) PreviewHideInImage(noteID string, imagePath string) Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	note, err := a.store.GetNote(noteID)
	if err != nil {
		return errorResp(err.Error())
	}

	// Encrypt note content before hiding
	encrypted, err := crypto.EncryptWithKey([]byte(note.Content), a.store.GetDEK())
	if err != nil {
		return errorResp(err.Error())
	}

	// Hide encrypted data in image
	resultBytes, err := stego.HideData(imagePath, []byte(encrypted))
	if err != nil {
		return errorResp(err.Error())
	}

	// Compute capacity info
	info, _ := os.Stat(imagePath)
	dataSize := len([]byte(encrypted))

	resultBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(resultBytes)
	return successResp(map[string]interface{}{
		"preview":   resultBase64,
		"data_size": fmt.Sprintf("%.2f KB", float64(dataSize)/1024),
		"img_size":  fmt.Sprintf("%.2f KB", float64(info.Size())/1024),
	})
}

// --- File Vault Methods ---

// GetFiles returns all encrypted file metadata
func (a *App) GetFiles() Response {
	files, err := a.store.GetFiles()
	if err != nil {
		return errorResp(err.Error())
	}
	result := make([]FileMetadataResponse, len(files))
	for i, f := range files {
		ff := f
		result[i] = fileMetaToResponse(&ff)
	}
	return successResp(result)
}

// AddFile opens a file picker, encrypts the file and stores it
func (a *App) AddFile() Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}

	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select File to Encrypt",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "All Files", Pattern: "*.*"},
			{DisplayName: "Documents", Pattern: "*.pdf;*.txt;*.md;*.docx"},
			{DisplayName: "Images", Pattern: "*.jpg;*.jpeg;*.png;*.gif"},
		},
	})
	if err != nil {
		return errorResp(err.Error())
	}
	if filePath == "" {
		return errorResp("No file selected")
	}

	meta, err := a.store.AddFile(filePath)
	if err != nil {
		return errorResp(err.Error())
	}
	return successResp(fileMetaToResponse(meta))
}

// ExportFile decrypts a file and saves it to user-chosen location
func (a *App) ExportFile(id string) Response {
	data, meta, err := a.store.DecryptFile(id)
	if err != nil {
		if err.Error() == store.ErrTampered.Error() {
			return Response{
				Success: false,
				Error:   "TAMPERED",
				Data:    fileMetaToResponse(meta),
			}
		}
		return errorResp(err.Error())
	}

	savePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Export Decrypted File",
		DefaultFilename: meta.OriginalName,
	})
	if err != nil {
		return errorResp(err.Error())
	}
	if savePath == "" {
		return errorResp("No location selected")
	}

	if err := os.WriteFile(savePath, data, 0644); err != nil {
		return errorResp(err.Error())
	}
	return successResp(map[string]string{"path": savePath})
}

// ExportFileForced decrypts and exports a file bypassing ECDSA signature check.
// Used for tamper-bypass recovery at the user's explicit request.
func (a *App) ExportFileForced(id string) Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}
	data, meta, err := a.store.DecryptFileForced(id)

	// Even if decryption fails (ErrDecryptFailed), data contains the raw encrypted bytes.
	// We still let the user save whatever we could recover.
	if err != nil && data == nil {
		return errorResp(err.Error())
	}

	warning := "exported without signature verification"
	defaultName := "recovered_" + id
	if meta != nil {
		defaultName = meta.OriginalName
	}
	if err != nil {
		// AES decryption failed — we're saving raw encrypted bytes
		warning = "AES decryption failed — raw encrypted content saved"
		defaultName = defaultName + ".corrupted"
	}

	savePath, err2 := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Export File (Bypassing Signature Check)",
		DefaultFilename: defaultName,
	})
	if err2 != nil {
		return errorResp(err2.Error())
	}
	if savePath == "" {
		return errorResp("No location selected")
	}

	if err2 := os.WriteFile(savePath, data, 0644); err2 != nil {
		return errorResp(err2.Error())
	}
	return successResp(map[string]interface{}{
		"path":      savePath,
		"warning":   warning,
		"corrupted": err != nil,
	})
}

// GetFileTamperDetails returns per-field tamper analysis for a file
func (a *App) GetFileTamperDetails(id string) Response {
	if !a.store.IsUnlocked() {
		return errorResp("Vault is locked")
	}
	details, meta, err := a.store.GetFileTamperDetails(id)
	if err != nil {
		return errorResp(err.Error())
	}
	return successResp(map[string]interface{}{
		"meta":    fileMetaToResponse(meta),
		"details": details,
	})
}

// DeleteFile removes an encrypted file from the vault
func (a *App) DeleteFile(id string) Response {
	if err := a.store.DeleteFile(id); err != nil {
		return errorResp(err.Error())
	}
	return successResp(true)
}

// VerifyFile verifies the ECDSA signature of a file's metadata
func (a *App) VerifyFile(id string) Response {
	_, meta, err := a.store.DecryptFile(id)
	if err != nil {
		if meta != nil && meta.Tampered {
			return Response{Success: false, Error: "TAMPERED", Data: fileMetaToResponse(meta)}
		}
		return errorResp(err.Error())
	}
	return successResp(fileMetaToResponse(meta))
}

// SearchNotes searches notes by title
func (a *App) SearchNotes(query string) Response {
	notes, err := a.store.SearchNotes(query)
	if err != nil {
		return errorResp(err.Error())
	}
	result := make([]NoteResponse, len(notes))
	for i, n := range notes {
		nn := n
		result[i] = noteToResponse(&nn)
	}
	return successResp(result)
}

// FileMetadataResponse is the JSON-serializable file metadata
type FileMetadataResponse struct {
	ID           string `json:"id"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	SizeHuman    string `json:"size_human"`
	ContentHash  string `json:"content_hash"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	Tampered     bool   `json:"tampered"`
}

func fileMetaToResponse(m *store.FileMetadata) FileMetadataResponse {
	if m == nil {
		return FileMetadataResponse{}
	}
	return FileMetadataResponse{
		ID:           m.ID,
		OriginalName: m.OriginalName,
		MimeType:     m.MimeType,
		Size:         m.Size,
		SizeHuman:    humanSize(m.Size),
		ContentHash:  m.ContentHash,
		CreatedAt:    m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    m.UpdatedAt.Format(time.RFC3339),
		Tampered:     m.Tampered,
	}
}

func humanSize(n int64) string {
	switch {
	case n >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
