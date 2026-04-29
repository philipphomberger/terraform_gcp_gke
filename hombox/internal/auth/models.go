package auth

import "time"

// RegisterRequest is the payload for username/password registration.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
}

// LoginRequest is the payload for password login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"`
}

// TOTPEnableResponse carries the secret and provisioning URI for setup.
type TOTPEnableResponse struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
	QRCode string `json:"qr_code"` // base64-encoded PNG
}

// TOTPVerifyRequest is used to confirm and enable TOTP.
type TOTPVerifyRequest struct {
	Code string `json:"code"`
}

// UserResponse is the public user representation returned by the API.
type UserResponse struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       *string   `json:"email,omitempty"`
	DisplayName *string   `json:"display_name,omitempty"`
	TOTPEnabled bool      `json:"totp_enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// SessionResponse is returned after successful authentication.
type SessionResponse struct {
	Token   string       `json:"token"`
	User    UserResponse `json:"user"`
	Expires time.Time    `json:"expires"`
}

// Context keys for storing auth info in request context.
type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	SessionToken contextKey = "session_token"
)
