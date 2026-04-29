package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/philipphomberger/hombox/internal/config"
)

// WebAuthnHandler handles WebAuthn (passkey) registration and login.
type WebAuthnHandler struct {
	pool     *pgxpool.Pool
	wa       *webauthn.WebAuthn
	authSvc  *Service
}

// NewWebAuthnHandler creates a WebAuthn handler.
func NewWebAuthnHandler(pool *pgxpool.Pool, authSvc *Service, cfg config.WebAuthnConfig) (*WebAuthnHandler, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: cfg.RPDisplayName,
		RPOrigins:     []string{cfg.RPOrigin},
	})
	if err != nil {
		return nil, fmt.Errorf("create webauthn: %w", err)
	}

	return &WebAuthnHandler{
		pool:    pool,
		wa:      wa,
		authSvc: authSvc,
	}, nil
}

// BeginRegister starts WebAuthn credential creation. Requires authenticated session.
func (h *WebAuthnHandler) BeginRegister(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	user, err := h.authSvc.GetUser(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	wu := &webAuthnUser{
		id:          []byte(user.ID),
		name:        user.Username,
		displayName: derefStr(user.DisplayName),
		credentials: []webauthn.Credential{}, // Don't exclude existing
	}

	// Load existing credentials to avoid duplicates
	rows, err := h.pool.Query(r.Context(),
		`SELECT credential_id, public_key, attestation_type, transports, sign_count
		 FROM webauthn_credentials WHERE user_id = $1`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var credID []byte
			var pubKey []byte
			var attType *string
			var transports []string
			var signCount uint32
			if err := rows.Scan(&credID, &pubKey, &attType, &transports, &signCount); err == nil {
				wu.credentials = append(wu.credentials, webauthn.Credential{
					ID:              credID,
					PublicKey:       pubKey,
					AttestationType: derefStr(attType),
					Transport:       parseTransports(transports),
				})
			}
		}
	}

	// Exclude existing credentials
	creation, session, err := h.wa.BeginRegistration(wu, webauthn.WithExclusions(wu.credentialDescriptors()))
	if err != nil {
		slog.Error("begin webauthn registration", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "webauthn registration failed"})
		return
	}

	// Store session data in Redis for retrieval during finish
	sessionData, _ := json.Marshal(session)
	h.authSvc.rdb.Client.Set(r.Context(), "webauthn:register:"+userID, sessionData, 0).Err()

	writeJSON(w, http.StatusOK, creation)
}

// FinishRegister completes WebAuthn credential creation.
func (h *WebAuthnHandler) FinishRegister(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(UserIDKey).(string)
	user, err := h.authSvc.GetUser(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	// Retrieve session
	sessionData, err := h.authSvc.rdb.Client.Get(r.Context(), "webauthn:register:"+userID).Result()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "registration session expired"})
		return
	}
	h.authSvc.rdb.Client.Del(r.Context(), "webauthn:register:"+userID)

	var session webauthn.SessionData
	json.Unmarshal([]byte(sessionData), &session)

	wu := &webAuthnUser{
		id:          []byte(user.ID),
		name:        user.Username,
		displayName: derefStr(user.DisplayName),
	}

	cred, err := h.wa.FinishRegistration(wu, session, r)
	if err != nil {
		slog.Error("finish webauthn registration", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "registration verification failed"})
		return
	}

	// Store credential in database
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, attestation_type, transports, sign_count)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, cred.ID, cred.PublicKey, cred.AttestationType, transportStrings(cred.Transport), cred.Authenticator.SignCount,
	)
	if err != nil {
		slog.Error("store webauthn credential", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store credential"})
		return
	}

	slog.Info("webauthn credential registered", "user_id", userID)
	writeJSON(w, http.StatusCreated, map[string]string{"status": "passkey_registered"})
}

// BeginLogin starts WebAuthn assertion (passwordless login).
func (h *WebAuthnHandler) BeginLogin(w http.ResponseWriter, r *http.Request) {
	assertion, session, err := h.wa.BeginDiscoverableLogin()
	if err != nil {
		slog.Error("begin webauthn login", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "webauthn login failed"})
		return
	}

	// Store session with a random challenge ID
	challengeID := generateToken(32)
	sessionData, _ := json.Marshal(session)
	h.authSvc.rdb.Client.Set(r.Context(), "webauthn:login:"+challengeID, sessionData, 0).Err()

	// Include the challenge ID so the client can pass it back
	response := map[string]interface{}{
		"challenge_id": challengeID,
		"public_key":   assertion.Response,
	}
	writeJSON(w, http.StatusOK, response)
}

// FinishLogin completes WebAuthn assertion.
func (h *WebAuthnHandler) FinishLogin(w http.ResponseWriter, r *http.Request) {
	// Extract challenge ID from query param
	challengeID := r.URL.Query().Get("challenge_id")
	if challengeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing challenge_id"})
		return
	}

	sessionData, err := h.authSvc.rdb.Client.Get(r.Context(), "webauthn:login:"+challengeID).Result()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "login session expired"})
		return
	}
	h.authSvc.rdb.Client.Del(r.Context(), "webauthn:login:"+challengeID)

	var session webauthn.SessionData
	json.Unmarshal([]byte(sessionData), &session)

	// Discoverable login — capture user ID from the handler closure
	var loginUserID string
	cred, err := h.wa.FinishDiscoverableLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		wu, err := h.findUserByCredential(r.Context(), rawID)
		if err == nil {
			loginUserID = string(wu.id)
		}
		return wu, err
	}, session, r)
	if err != nil {
		slog.Error("finish webauthn login", "error", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
		return
	}

	// Update sign count
	_, _ = h.pool.Exec(r.Context(),
		`UPDATE webauthn_credentials SET sign_count = $1 WHERE credential_id = $2`,
		cred.Authenticator.SignCount, cred.ID)

	user, err := h.authSvc.GetUser(r.Context(), loginUserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user lookup failed"})
		return
	}

	sessionResp, err := h.authSvc.createSession(r.Context(), user.ID, user.Username,
		derefStr(user.Email), derefStr(user.DisplayName),
		user.TOTPEnabled, user.CreatedAt, r.UserAgent(), clientIP(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session creation failed"})
		return
	}

	maxAge := int(h.authSvc.cfg.Session.MaxAge.Seconds())
	SetSessionCookie(w, sessionResp.Token, maxAge, false)
	writeJSON(w, http.StatusOK, sessionResp)
}

// findUserByCredential looks up a user by their WebAuthn credential ID.
func (h *WebAuthnHandler) findUserByCredential(ctx context.Context, credentialID []byte) (*webAuthnUser, error) {
	var userID, username string
	var displayName *string
	err := h.pool.QueryRow(ctx,
		`SELECT u.id, u.username, u.display_name
		 FROM users u JOIN webauthn_credentials wc ON wc.user_id = u.id
		 WHERE wc.credential_id = $1`,
		credentialID,
	).Scan(&userID, &username, &displayName)
	if err != nil {
		return nil, fmt.Errorf("credential not found")
	}

	// Load all credentials
	creds := []webauthn.Credential{}
	rows, err := h.pool.Query(ctx,
		`SELECT credential_id, public_key, attestation_type, transports, sign_count
		 FROM webauthn_credentials WHERE user_id = $1`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cid, pk []byte
			var at *string
			var tr []string
			var sc uint32
			if rows.Scan(&cid, &pk, &at, &tr, &sc) == nil {
				creds = append(creds, webauthn.Credential{
					ID:              cid,
					PublicKey:       pk,
					AttestationType: derefStr(at),
					Transport:       parseTransports(tr),
				})
			}
		}
	}

	return &webAuthnUser{
		id:          []byte(userID),
		name:        username,
		displayName: derefStr(displayName),
		credentials: creds,
	}, nil
}

// webAuthnUser implements the webauthn.User interface.
type webAuthnUser struct {
	id          []byte
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte                { return u.id }
func (u *webAuthnUser) WebAuthnName() string              { return u.name }
func (u *webAuthnUser) WebAuthnDisplayName() string       { return u.displayName }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (u *webAuthnUser) credentialDescriptors() []protocol.CredentialDescriptor {
	descs := make([]protocol.CredentialDescriptor, len(u.credentials))
	for i, cred := range u.credentials {
		descs[i] = protocol.CredentialDescriptor{
			Type:            protocol.PublicKeyCredentialType,
			CredentialID:    cred.ID,
			Transport:       cred.Transport,
		}
	}
	return descs
}

func parseTransports(ts []string) []protocol.AuthenticatorTransport {
	out := make([]protocol.AuthenticatorTransport, len(ts))
	for i, t := range ts {
		out[i] = protocol.AuthenticatorTransport(t)
	}
	return out
}

func transportStrings(ts []protocol.AuthenticatorTransport) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t)
	}
	return out
}
