package auth

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"github.com/philipphomberger/hombox/internal/cache"
	"github.com/philipphomberger/hombox/internal/config"
)

// OIDCHandler handles OpenID Connect authentication flows.
type OIDCHandler struct {
	pool    *pgxpool.Pool
	rdb     *cache.Redis
	cfg     *config.Config
	authSvc *Service
}

// NewOIDCHandler creates a new OIDC handler.
func NewOIDCHandler(pool *pgxpool.Pool, rdb *cache.Redis, cfg *config.Config, authSvc *Service) *OIDCHandler {
	return &OIDCHandler{
		pool:    pool,
		rdb:     rdb,
		cfg:     cfg,
		authSvc: authSvc,
	}
}

// knownProviders maps short names to their OIDC issuer URLs.
var knownProviders = map[string]string{
	"google": "https://accounts.google.com",
	"github": "https://github.com/login/oauth", // GitHub uses OAuth2, handled separately
}

// Begin initiates the OIDC redirect flow.
func (h *OIDCHandler) Begin(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}

	clientCfg := h.cfg.GetOIDCClientConfig(provider)
	if clientCfg.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider not configured: " + provider})
		return
	}

	issuerURL, ok := knownProviders[provider]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider: " + provider})
		return
	}

	ctx := r.Context()
	p, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		slog.Error("create oidc provider", "provider", provider, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create provider"})
		return
	}

	oauthCfg := &oauth2.Config{
		ClientID:     clientCfg.ClientID,
		ClientSecret: clientCfg.ClientSecret,
		RedirectURL:  clientCfg.RedirectURL,
		Endpoint:     p.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	// Generate state token to prevent CSRF
	state := generateToken(32)
	h.rdb.Client.Set(ctx, "oidc:state:"+state, provider, 0).Err()

	// Store nonce for ID token verification
	nonce := generateToken(32)
	h.rdb.Client.Set(ctx, "oidc:nonce:"+state, nonce, 0).Err()

	authURL := oauthCfg.AuthCodeURL(state, oidc.Nonce(nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback handles the OIDC provider callback after authorization.
func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	provider := r.URL.Query().Get("provider")

	if state == "" || code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing state or code"})
		return
	}

	// Verify state
	ctx := r.Context()
	storedProvider, err := h.rdb.Client.Get(ctx, "oidc:state:"+state).Result()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid state"})
		return
	}
	h.rdb.Client.Del(ctx, "oidc:state:"+state)

	if provider != "" {
		storedProvider = provider
	}

	clientCfg := h.cfg.GetOIDCClientConfig(storedProvider)
	issuerURL := knownProviders[storedProvider]

	p, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		slog.Error("create oidc provider in callback", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "provider error"})
		return
	}

	oauthCfg := &oauth2.Config{
		ClientID:     clientCfg.ClientID,
		ClientSecret: clientCfg.ClientSecret,
		RedirectURL:  clientCfg.RedirectURL,
		Endpoint:     p.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	// Exchange code for token
	oauthToken, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		slog.Error("exchange oidc code", "error", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token exchange failed"})
		return
	}

	// Extract raw ID token
	rawIDToken, ok := oauthToken.Extra("id_token").(string)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no id_token in response"})
		return
	}

	// Verify ID token
	verifier := p.Verifier(&oidc.Config{ClientID: clientCfg.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		slog.Error("verify id token", "error", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "id token verification failed"})
		return
	}

	// Extract claims
	var claims struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to parse claims"})
		return
	}

	// Find existing OIDC connection or create new user
	var userID string
	err = h.pool.QueryRow(ctx,
		`SELECT user_id FROM oidc_connections WHERE provider = $1 AND subject = $2`,
		storedProvider, claims.Subject,
	).Scan(&userID)

	if err != nil {
		// No existing connection — create user
		username := strings.ToLower(strings.ReplaceAll(claims.Name, " ", "_"))
		if username == "" {
			username = storedProvider + "_" + claims.Subject[:8]
		}

		// Try to create user
		err = h.pool.QueryRow(ctx,
			`INSERT INTO users (username, email, display_name)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (username) DO UPDATE SET username = users.username
			 RETURNING id`,
			username, claims.Email, claims.Name,
		).Scan(&userID)

		if err != nil {
			slog.Error("create oidc user", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create user"})
			return
		}

		// Create OIDC connection
		_, _ = h.pool.Exec(ctx,
			`INSERT INTO oidc_connections (user_id, provider, subject) VALUES ($1, $2, $3)
			 ON CONFLICT (provider, subject) DO NOTHING`,
			userID, storedProvider, claims.Subject)
	}

	// Get user for session
	user, err := h.authSvc.GetUser(ctx, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user lookup failed"})
		return
	}

	session, err := h.authSvc.createSession(ctx, user.ID, user.Username,
		derefStr(user.Email), derefStr(user.DisplayName),
		user.TOTPEnabled, user.CreatedAt, r.UserAgent(), clientIP(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session creation failed"})
		return
	}

	maxAge := int(h.authSvc.cfg.Session.MaxAge.Seconds())
	SetSessionCookie(w, session.Token, maxAge, false)

	// Redirect to frontend dashboard after successful login
	frontendURL := h.cfg.BaseURL
	http.Redirect(w, r, frontendURL+"/dashboard", http.StatusFound)
}
