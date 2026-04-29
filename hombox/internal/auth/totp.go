package auth

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"

	"github.com/pquerna/otp/totp"
)

// TOTPService handles TOTP operations.
type TOTPService struct{}

// NewTOTPService creates a new TOTP service.
func NewTOTPService() *TOTPService {
	return &TOTPService{}
}

// Generate creates a new TOTP key for a user.
func (s *TOTPService) Generate(accountName, issuer string) (*TOTPEnableResponse, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
	})
	if err != nil {
		return nil, fmt.Errorf("generate totp key: %w", err)
	}

	// Generate QR code as PNG and base64-encode
	qr, err := key.Image(200, 200)
	if err != nil {
		return nil, fmt.Errorf("generate QR image: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, qr); err != nil {
		return nil, fmt.Errorf("encode QR PNG: %w", err)
	}

	return &TOTPEnableResponse{
		Secret: key.Secret(),
		URI:    key.URL(),
		QRCode: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
	}, nil
}

// Verify checks a TOTP code against the secret.
func (s *TOTPService) Verify(secret, code string) error {
	if !totp.Validate(code, secret) {
		return fmt.Errorf("invalid totp code")
	}
	return nil
}
