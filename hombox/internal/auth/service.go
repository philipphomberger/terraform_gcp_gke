package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/philipphomberger/hombox/internal/cache"
	"github.com/philipphomberger/hombox/internal/config"
)

// Service handles authentication business logic.
type Service struct {
	pool *pgxpool.Pool
	rdb  *cache.Redis
	cfg  AuthConfig
}

// AuthConfig is the subset of config the auth service needs.
type AuthConfig struct {
	Session  config.SessionConfig
	WebAuthn config.WebAuthnConfig
}

// NewService creates a new auth service.
func NewService(pool *pgxpool.Pool, rdb *cache.Redis, cfg AuthConfig) *Service {
	return &Service{pool: pool, rdb: rdb, cfg: cfg}
}

// Register creates a new user with a bcrypt-hashed password.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*UserResponse, error) {
	if len(req.Username) < 3 || len(req.Username) > 64 {
		return nil, fmt.Errorf("username must be 3-64 characters")
	}
	if len(req.Password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Convert empty email to nil for nullable column
	var email any = req.Email
	if req.Email == "" {
		email = nil
	}

	var user UserResponse
	err = s.pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, email) VALUES ($1, $2, $3)
		 ON CONFLICT (username) DO NOTHING
		 RETURNING id, username, email, display_name, totp_enabled, created_at`,
		req.Username, string(hash), email,
	).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.TOTPEnabled, &user.CreatedAt)

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, fmt.Errorf("username already taken")
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}

	slog.Info("user registered", "user_id", user.ID, "username", user.Username)
	return &user, nil
}

// Login verifies a password and returns a session.
func (s *Service) Login(ctx context.Context, req LoginRequest, userAgent, ip string) (*SessionResponse, error) {
	var user struct {
		ID           string
		Username     string
		Email        *string
		DisplayName  *string
		TOTPEnabled  bool
		TOTPSecret   *string
		PasswordHash string
		CreatedAt    time.Time
	}

	err := s.pool.QueryRow(ctx,
		`SELECT id, username, email, display_name, totp_enabled, totp_secret, password_hash, created_at
		 FROM users WHERE username = $1`,
		req.Username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName,
		&user.TOTPEnabled, &user.TOTPSecret, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid username or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("invalid username or password")
	}

	// TOTP challenge if enabled
	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			return nil, fmt.Errorf("totp_required")
		}
		totpSvc := NewTOTPService()
		if err := totpSvc.Verify(*user.TOTPSecret, req.TOTPCode); err != nil {
			return nil, fmt.Errorf("invalid totp code")
		}
	}

	return s.createSession(ctx, user.ID, user.Username,
		derefStr(user.Email), derefStr(user.DisplayName),
		user.TOTPEnabled, user.CreatedAt, userAgent, ip)
}

// Logout invalidates the current session.
func (s *Service) Logout(ctx context.Context, token string) error {
	if err := s.rdb.DeleteSession(ctx, token); err != nil {
		return fmt.Errorf("delete session from redis: %w", err)
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// ValidateSession checks if a session token is valid and returns the user ID.
func (s *Service) ValidateSession(ctx context.Context, token string) (string, error) {
	// fast path: Redis
	userID, err := s.rdb.GetSession(ctx, token)
	if err != nil {
		return "", fmt.Errorf("redis get session: %w", err)
	}
	if userID != "" {
		return userID, nil
	}

	// slow path: PostgreSQL (e.g. after Redis restart)
	var uid string
	var expires time.Time
	err = s.pool.QueryRow(ctx,
		`SELECT user_id, expires_at FROM sessions WHERE token = $1`, token,
	).Scan(&uid, &expires)
	if err != nil {
		return "", fmt.Errorf("session not found")
	}
	if time.Now().After(expires) {
		return "", fmt.Errorf("session expired")
	}

	// repopulate Redis
	_ = s.rdb.SetSession(ctx, token, uid, time.Until(expires))
	return uid, nil
}

// EnableTOTP generates a TOTP secret for a user and returns setup info.
func (s *Service) EnableTOTP(ctx context.Context, userID, issuer string) (*TOTPEnableResponse, error) {
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	totpSvc := NewTOTPService()
	resp, err := totpSvc.Generate(user.Username, issuer)
	if err != nil {
		return nil, fmt.Errorf("generate totp: %w", err)
	}

	// Store secret temporarily (not enabled until verified)
	_, err = s.pool.Exec(ctx, `UPDATE users SET totp_secret = $1 WHERE id = $2`, resp.Secret, userID)
	if err != nil {
		return nil, fmt.Errorf("store totp secret: %w", err)
	}

	return resp, nil
}

// VerifyTOTP confirms a TOTP code and enables TOTP for the user.
func (s *Service) VerifyTOTP(ctx context.Context, userID, code string) error {
	var secret string
	err := s.pool.QueryRow(ctx, `SELECT totp_secret FROM users WHERE id = $1`, userID).Scan(&secret)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	if secret == "" {
		return fmt.Errorf("totp not initiated")
	}

	totpSvc := NewTOTPService()
	if err := totpSvc.Verify(secret, code); err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `UPDATE users SET totp_enabled = true WHERE id = $1`, userID)
	return err
}

// DisableTOTP removes TOTP from a user account.
func (s *Service) DisableTOTP(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET totp_enabled = false, totp_secret = NULL WHERE id = $1`, userID)
	return err
}

// GetUser returns a user by ID.
func (s *Service) GetUser(ctx context.Context, userID string) (*UserResponse, error) {
	var u UserResponse
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, email, display_name, totp_enabled, created_at FROM users WHERE id = $1`,
		userID,
	).Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.TOTPEnabled, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	return &u, nil
}

// ---- internal helpers ----

func (s *Service) createSession(ctx context.Context,
	userID, username, email, displayName string, totpEnabled bool, createdAt time.Time,
	userAgent, ip string,
) (*SessionResponse, error) {
	token := generateToken(64)
	expires := time.Now().Add(s.cfg.Session.MaxAge)

	// store session token with user ID
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (user_id, token, user_agent, ip_address, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, token, userAgent, ip, expires)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	if err := s.rdb.SetSession(ctx, token, userID, s.cfg.Session.MaxAge); err != nil {
		return nil, fmt.Errorf("cache session: %w", err)
	}

	ur := UserResponse{
		ID:          userID,
		Username:    username,
		Email:       &email,
		DisplayName: &displayName,
		TOTPEnabled: totpEnabled,
		CreatedAt:   createdAt,
	}
	if email == "" {
		ur.Email = nil
	}
	if displayName == "" {
		ur.DisplayName = nil
	}

	return &SessionResponse{
		Token:   token,
		User:    ur,
		Expires: expires,
	}, nil
}

// SetSessionCookie sets the HTTP-only session cookie. maxAge in seconds.
func SetSessionCookie(w http.ResponseWriter, token string, maxAge int, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "hombox_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "hombox_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// GetSessionCookie extracts the session token from the cookie.
func GetSessionCookie(r *http.Request) string {
	cookie, err := r.Cookie("hombox_session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func generateToken(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
