package totp

import (
	"encoding/base64"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateSecret generates a new TOTP secret for the user
func GenerateSecret(accountName string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "SecretVault",
		AccountName: accountName,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
	})
	if err != nil {
		return "", "", err
	}

	// Convert QR code image to base64
	img, err := key.Image(200, 200)
	if err != nil {
		return key.Secret(), "", err
	}

	// Encode image to PNG then base64
	var buf []byte
	buf, err = encodePNG(img)
	if err != nil {
		return key.Secret(), "", err
	}

	qrBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf)
	return key.Secret(), qrBase64, nil
}

// ValidateCode validates a TOTP code against the secret
func ValidateCode(code string, secret string) bool {
	return totp.Validate(code, secret)
}
