package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port     string
	BaseURL  string
	Env      string
	Database DatabaseConfig
	Redis    RedisConfig
	S3       S3Config
	Session  SessionConfig
	WebAuthn WebAuthnConfig
	Upload   UploadConfig
	OIDC     OIDCConfig
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	URL string
}

type S3Config struct {
	Endpoint        string
	AccessKey       string
	SecretKey       string
	Bucket          string
	DerivativesBucket string
	UseSSL          bool
}

type SessionConfig struct {
	Secret  string
	MaxAge  time.Duration
}

type WebAuthnConfig struct {
	RPID          string
	RPOrigin      string
	RPDisplayName string
}

type OIDCConfig struct {
	Providers []string
	// Per-provider config accessed via GetOIDCClientConfig
}

type UploadConfig struct {
	MaxSimpleSize     int64
	MaxMultipartSize  int64
	AnonymousMaxSize  int64
	ChunkSize         int64
}

type OIDCClientConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:    envOrDefault("HOMBOX_PORT", "8080"),
		BaseURL: envOrDefault("HOMBOX_BASE_URL", "http://localhost:8080"),
		Env:     envOrDefault("HOMBOX_ENV", "development"),
		Database: DatabaseConfig{
			URL:             requireEnv("DATABASE_URL"),
			MaxOpenConns:    envIntOrDefault("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    envIntOrDefault("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: time.Duration(envIntOrDefault("DB_CONN_MAX_LIFETIME_SEC", 300)) * time.Second,
		},
		Redis: RedisConfig{
			URL: requireEnv("REDIS_URL"),
		},
		S3: S3Config{
			Endpoint:         requireEnv("S3_ENDPOINT"),
			AccessKey:        requireEnv("S3_ACCESS_KEY"),
			SecretKey:        requireEnv("S3_SECRET_KEY"),
			Bucket:           envOrDefault("S3_BUCKET", "hombox-files"),
			DerivativesBucket: envOrDefault("S3_DERIVATIVES_BUCKET", "hombox-derivatives"),
			UseSSL:           envBoolOrDefault("S3_USE_SSL", false),
		},
		Session: SessionConfig{
			Secret: requireEnv("SESSION_SECRET"),
			MaxAge: time.Duration(envIntOrDefault("SESSION_MAX_AGE", 604800)) * time.Second,
		},
		WebAuthn: WebAuthnConfig{
			RPID:          envOrDefault("WEBAUTHN_RP_ID", "localhost"),
			RPOrigin:      envOrDefault("WEBAUTHN_RP_ORIGIN", "http://localhost:8080"),
			RPDisplayName: envOrDefault("WEBAUTHN_RP_DISPLAY_NAME", "Hombox"),
		},
		OIDC: OIDCConfig{
			Providers: strings.Split(envOrDefault("OIDC_PROVIDERS", ""), ","),
		},
		Upload: UploadConfig{
			MaxSimpleSize:    int64(envIntOrDefault("UPLOAD_MAX_SIMPLE_SIZE", 104857600)),
			MaxMultipartSize: int64(envIntOrDefault("UPLOAD_MAX_MULTIPART_SIZE", 53687091200)),
			AnonymousMaxSize: int64(envIntOrDefault("UPLOAD_ANONYMOUS_MAX_SIZE", 524288000)),
			ChunkSize:        int64(envIntOrDefault("UPLOAD_CHUNK_SIZE", 5242880)),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) GetOIDCClientConfig(provider string) OIDCClientConfig {
	return OIDCClientConfig{
		ClientID:     os.Getenv(fmt.Sprintf("OIDC_%s_CLIENT_ID", strings.ToUpper(provider))),
		ClientSecret: os.Getenv(fmt.Sprintf("OIDC_%s_CLIENT_SECRET", strings.ToUpper(provider))),
		RedirectURL:  fmt.Sprintf("%s/api/auth/oidc/callback?provider=%s", c.BaseURL, provider),
	}
}

func (c *Config) validate() error {
	if c.Env == "production" && c.Session.Secret == "change-me-to-a-random-64-char-string" {
		return fmt.Errorf("SESSION_SECRET must be changed in production")
	}
	return nil
}

func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

// helpers

func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic(fmt.Sprintf("required env var %s is not set", key))
	}
	return val
}

func envOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func envIntOrDefault(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

func envBoolOrDefault(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}
