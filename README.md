# Secret Vault

Personal security desktop application — store confidential notes, files, and hide data in images with multiple layers of encryption.

---

## Features

### Authentication & Vault
- Create a vault protected by a master password
- Derive encryption key using **PBKDF2-SHA256** with 600,000 iterations
- Option to enable **TOTP 2FA** (Google Authenticator / Authy) — 6 separate input fields
- Manual vault locking; Vault locks automatically when the application is closed.
- Password change requires confirmation.

### Encrypted Notes
- Create **Markdown** formatted notes with toolbar (bold, italic, heading, code, list, blockquote)
- Direct **Edit/Preview** switching mode
- Full-text search by title and content (300 ms debounce)
- Attach **tags** to each note
- All content is encrypted with **AES-256-GCM** before writing to disk.

### File Vault
- Import any file into the vault — the file is encrypted with AES-256-GCM and saved as `~/.secretvault/files/<uuid>.enc`
- Each file is **digitally signed with ECDSA P-256** metadata (name, SHA-256 hash, MIME type, size)
- **Tamper detection**: immediately warns if file metadata is modified outside the application. Uses
- Export files using the save file dialog box
- Securely delete files from the vault

### Steganography (LSB)
- **Hide notes in PNG images**: AES-256-GCM encryption of note content and embedding it into the lowest bits (LSB) of the R/G/B color channel
- **Comparison preview**: Displays the original and resulting images side-by-side — indistinguishable to the naked eye
- **Extract & decode**: Select a PNG image, the application reads the LSB and decrypts it using AES-256-GCM to recover the original content
- Results are viewed as **Raw text** or **Markdown preview**
- Export steganographic images via the save file dialog box

### Backup
- Export the entire vault as an encrypted backup file + ECDSA digital signature
- Import the backup to restore data

---

## Security

| Components | Algorithms |
|---|---|
| Data encryption | AES-256-GCM |
| Key Derivation | PBKDF2-SHA256 · 600.000 round · salt 32 byte |
| Metadata Signing | ECDSA P-256 · SHA-256 |
| Two-Factor Authentication| TOTP (RFC 6238) · HMAC-SHA1 |
| Steganography | LSB 1-bit per channel · PNG lossless |
| Vault Format | JSON encrypted at `~/.secretvault/vault.json` (chmod 0600) |
| Key ECDSA | Private key encrypted with AES-256-GCM, stored in vault |

---


## Development
- Make sure that you have `go` and `npm` ready:
```bash
go version
# 1.24

npm -v
# 10.9.3
```

-  Dev mode
```bash
wails dev -tags webkit2_41 # Linux (Ubuntu 24.04+)
wails dev # macOS / Windows
```


- Build production

```bash
# Linux (Ubuntu 24.04+)
wails build -tags webkit2_41

# macOS / Windows
wails build 

# Windows
export PATH=$PATH:$(go env GOPATH)/bin && CGO_ENABLED=1 wails build -platform windows/amd64 -nsis 2>&1; echo "EXIT:$?" 

```

The Vault is stored at `~/.secretvault/vault.json`. The encrypted file is at `~/.secretvault/files/`.

## Todo:
The ChangePassword or Forget password feature will later apply blockchain encryption:
Users can enter a privateKey or mnemonic phrase