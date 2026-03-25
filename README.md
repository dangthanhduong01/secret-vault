# Secret Vault

Ứng dụng desktop bảo mật cá nhân — lưu trữ ghi chú mật, tệp tin và ẩn dữ liệu vào hình ảnh với nhiều lớp mã hóa.

---

## Tính năng

### 🔐 Xác thực & Vault
- Tạo vault được bảo vệ bằng mật khẩu chính (master password)
- Dẫn xuất khóa mã hóa bằng **PBKDF2-SHA256** với 600.000 vòng lặp
- Tùy chọn bật **TOTP 2FA** (Google Authenticator / Authy) — giao diện nhập 6 ô riêng biệt
- Khóa vault thủ công; vault tự khóa khi đóng ứng dụng
- Đổi mật khẩu có xác nhận lại

### 📝 Ghi chú mã hóa
- Soạn thảo ghi chú định dạng **Markdown** với toolbar (bold, italic, heading, code, list, blockquote)
- Chế độ chuyển đổi **Edit / Preview** trực tiếp
- Tìm kiếm full-text theo tiêu đề và nội dung (debounce 300 ms)
- Gắn **tags** cho từng ghi chú
- Toàn bộ nội dung được mã hóa **AES-256-GCM** trước khi ghi vào disk

### 📁 File Vault
- Nhập bất kỳ tệp tin nào vào vault — file được mã hóa AES-256-GCM và lưu dưới dạng `~/.secretvault/files/<uuid>.enc`
- Mỗi file được **ký số ECDSA P-256** trên metadata (tên, hash SHA-256, MIME type, kích thước)
- Phát hiện **tamper**: cảnh báo ngay nếu metadata file bị chỉnh sửa ngoài ứng dụng
- Xuất file ra ngoài với hộp thoại lưu tệp
- Xóa file khỏi vault an toàn

### 🖼️ Steganography (LSB)
- **Ẩn ghi chú vào hình ảnh PNG**: mã hóa AES-256-GCM nội dung ghi chú rồi nhúng vào các bit thấp nhất (LSB) của kênh màu R/G/B
- **Xem trước so sánh**: hiển thị ảnh gốc và ảnh kết quả cạnh nhau — không thể phân biệt bằng mắt thường
- **Trích xuất & giải mã**: chọn ảnh PNG, ứng dụng đọc LSB và giải mã AES-256-GCM để phục hồi nội dung gốc
- Kết quả xem dạng **Raw text** hoặc **Markdown preview**
- Xuất ảnh steganographic qua hộp thoại lưu tệp

### 💾 Backup
- Xuất toàn bộ vault thành file backup được mã hóa + ký số ECDSA
- Nhập backup để khôi phục dữ liệu

---

## Bảo mật

| Thành phần | Thuật toán |
|---|---|
| Mã hóa dữ liệu | AES-256-GCM |
| Dẫn xuất khóa | PBKDF2-SHA256 · 600.000 vòng · salt 32 byte |
| Ký số metadata | ECDSA P-256 · SHA-256 |
| Xác thực 2 yếu tố | TOTP (RFC 6238) · HMAC-SHA1 |
| Steganography | LSB 1-bit per channel · PNG lossless |
| Định dạng vault | JSON mã hóa tại `~/.secretvault/vault.json` (chmod 0600) |
| Khóa ECDSA | Private key được mã hóa AES-256-GCM, lưu trong vault |

---

## Cấu trúc dự án

```
secretvault/
├── main.go                  # Entrypoint Wails
├── app.go                   # Toàn bộ API backend (28 phương thức)
├── internal/
│   ├── crypto/
│   │   ├── kdf.go           # PBKDF2: GenerateSalt, DeriveKey, HashPassword
│   │   └── encrypt.go       # AES-256-GCM: Encrypt, Decrypt
│   ├── signature/
│   │   └── signature.go     # ECDSA P-256: GenerateKeyPair, Sign, Verify
│   ├── stego/
│   │   └── stego.go         # LSB steganography: HideData, ExtractData
│   ├── store/
│   │   ├── store.go         # Vault store: notes, files, auth
│   │   └── helpers.go       # Tiện ích store
│   └── totp/
│       ├── totp.go          # TOTP: setup, verify, enable/disable
│       └── png.go           # Tạo QR code PNG cho TOTP
└── frontend/
    └── src/
        └── views/
            ├── AuthView.vue      # Đăng nhập / Tạo vault / TOTP 2FA
            ├── VaultView.vue     # Ghi chú + File vault
            ├── StegoView.vue     # Steganography
            └── SettingsView.vue  # Cài đặt, backup, TOTP
```

---

## Stack

- **Backend**: Go 1.24 · Wails v2.11
- **Frontend**: Vue 3.5 · TypeScript · Tailwind CSS v4 · Vite 6
- **Build** (Ubuntu 24.04): `wails build -tags webkit2_41`
- **Build** (macOS / Windows): `wails build`

## Phát triển

```bash
# Chế độ dev (hot reload)
wails dev -tags webkit2_41   # Linux (Ubuntu 24.04+)
wails dev                    # macOS / Windows

# Build production
wails build -tags webkit2_41   # Linux (Ubuntu 24.04+)
wails build                    # macOS / Windows
export PATH=$PATH:$(go env GOPATH)/bin && CGO_ENABLED=1 wails build -platform windows/amd64 -nsis 2>&1; echo "EXIT:$?" # Windows
```

Vault được lưu tại `~/.secretvault/vault.json`. File mã hóa tại `~/.secretvault/files/`.


## Mở rộng sau:
Tính năng ChangePassword hay Forget password sau này sẽ áp dụng mã hóa blockchain:
Người dùng có thể nhập privateKey hoặc mnemonic phrase