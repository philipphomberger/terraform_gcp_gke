DROP INDEX IF EXISTS idx_sessions_user_id;
DROP INDEX IF EXISTS idx_sessions_token;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS oidc_connections;
DROP TABLE IF EXISTS webauthn_credentials;
DROP TABLE IF EXISTS users;
