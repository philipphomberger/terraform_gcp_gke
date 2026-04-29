package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/philipphomberger/hombox/internal/auth"
	"github.com/philipphomberger/hombox/internal/cache"
	"github.com/philipphomberger/hombox/internal/config"
	"github.com/philipphomberger/hombox/internal/database"
	"github.com/philipphomberger/hombox/internal/files"
	"github.com/philipphomberger/hombox/internal/sharing"
)

func main() {
	// CLI flags
	migrateUp := flag.Bool("migrate-up", false, "run database migrations up")
	migrateDown := flag.Bool("migrate-down", false, "rollback last migration")
	flag.Parse()

	// Load .env file (ignore error if missing — Docker uses real env vars)
	_ = godotenv.Load()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Structured JSON logging for production, text for dev
	if cfg.IsProduction() {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	// Run migrations and exit
	if *migrateUp {
		if err := database.RunMigrations(cfg.Database.URL); err != nil {
			slog.Error("migrate up failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if *migrateDown {
		if err := database.RunMigrationsDown(cfg.Database.URL); err != nil {
			slog.Error("migrate down failed", "error", err)
			os.Exit(1)
		}
		return
	}

	// Connect to PostgreSQL
	pool, err := database.Connect(cfg.Database)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Auto-run migrations in dev mode
	if !cfg.IsProduction() {
		if err := database.RunMigrations(cfg.Database.URL); err != nil {
			slog.Error("auto-migration failed", "error", err)
			os.Exit(1)
		}
	}

	// Connect to Redis
	rdb, err := cache.Connect(cfg.Redis)
	if err != nil {
		slog.Error("redis connection failed", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()

	// ---- Auth setup ----
	authCfg := auth.AuthConfig{
		Session:  cfg.Session,
		WebAuthn: cfg.WebAuthn,
	}
	authHandler := auth.NewHandler(pool, rdb, authCfg)
	authSvc := authHandler.Service()
	authMW := auth.Middleware(authSvc)

	// WebAuthn handler
	waHandler, err := auth.NewWebAuthnHandler(pool, authSvc, cfg.WebAuthn)
	if err != nil {
		slog.Error("webauthn setup failed", "error", err)
		os.Exit(1)
	}

	// OIDC handler
	oidcHandler := auth.NewOIDCHandler(pool, rdb, cfg, authSvc)

	// ---- File storage ----
	fileStorage, err := files.Connect(cfg.S3)
	if err != nil {
		slog.Error("file storage setup failed", "error", err)
		os.Exit(1)
	}
	fileSvc := files.NewService(pool, fileStorage, cfg.Upload)
	fileHandler := files.NewHandler(fileSvc)

	// ---- Sharing ----
	shareSvc := sharing.NewService(pool)
	shareHandler := sharing.NewHandler(shareSvc, fileSvc, fileStorage)

	// ---- Routes ----
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /api/health", healthHandler(pool, rdb))

	// --- Public auth routes ---
	mux.HandleFunc("POST /api/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/auth/logout", authHandler.Logout)

	// WebAuthn (public login, protected registration)
	mux.HandleFunc("GET /api/auth/webauthn/begin-login", waHandler.BeginLogin)
	mux.HandleFunc("POST /api/auth/webauthn/finish-login", waHandler.FinishLogin)
	mux.Handle("POST /api/auth/webauthn/begin-register", authMW(http.HandlerFunc(waHandler.BeginRegister)))
	mux.Handle("POST /api/auth/webauthn/finish-register", authMW(http.HandlerFunc(waHandler.FinishRegister)))

	// OIDC (public)
	mux.HandleFunc("GET /api/auth/oidc/begin", oidcHandler.Begin)
	mux.HandleFunc("GET /api/auth/oidc/callback", oidcHandler.Callback)

	// --- Protected auth routes (session required) ---
	mux.Handle("GET /api/auth/me", authMW(http.HandlerFunc(authHandler.Me)))
	mux.Handle("POST /api/auth/totp/enable", authMW(http.HandlerFunc(authHandler.EnableTOTP)))
	mux.Handle("POST /api/auth/totp/verify", authMW(http.HandlerFunc(authHandler.VerifyTOTP)))
	mux.Handle("POST /api/auth/totp/disable", authMW(http.HandlerFunc(authHandler.DisableTOTP)))

	// --- Protected share routes (session required) ---
	mux.Handle("POST /api/shares", authMW(http.HandlerFunc(shareHandler.Create)))
	mux.Handle("GET /api/shares", authMW(http.HandlerFunc(shareHandler.List)))
	mux.Handle("DELETE /api/shares/{id}", authMW(http.HandlerFunc(shareHandler.Delete)))

	// --- Public share routes (no auth) ---
	mux.HandleFunc("GET /api/shares/{token}", shareHandler.Get)
	mux.HandleFunc("GET /api/shares/{token}/files/{id}/download", shareHandler.Download)
	mux.HandleFunc("POST /api/shares/{token}/upload", shareHandler.AnonymousUpload)
	mux.HandleFunc("POST /api/shares/{token}/upload/complete", shareHandler.AnonymousConfirmUpload)

	// --- Protected file routes (session required) ---
	mux.Handle("POST /api/files/upload", authMW(http.HandlerFunc(fileHandler.InitiateUpload)))
	mux.Handle("POST /api/files/upload/complete", authMW(http.HandlerFunc(fileHandler.ConfirmUpload)))
	mux.Handle("GET /api/files/{id}/download", authMW(http.HandlerFunc(fileHandler.Download)))
	mux.Handle("GET /api/files/{id}/preview", authMW(http.HandlerFunc(fileHandler.Preview)))
	mux.Handle("POST /api/files/folder", authMW(http.HandlerFunc(fileHandler.CreateFolder)))
	mux.Handle("GET /api/files", authMW(http.HandlerFunc(fileHandler.List)))
	mux.Handle("GET /api/files/search", authMW(http.HandlerFunc(fileHandler.Search)))
	mux.Handle("PATCH /api/files/{id}/rename", authMW(http.HandlerFunc(fileHandler.Rename)))
	mux.Handle("PATCH /api/files/{id}/move", authMW(http.HandlerFunc(fileHandler.Move)))
	mux.Handle("DELETE /api/files/{id}", authMW(http.HandlerFunc(fileHandler.Delete)))

	// Catch-all 404
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      withMiddleware(mux, loggingMiddleware, corsMiddleware(cfg)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		slog.Info("shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	slog.Info("server starting", "port", cfg.Port, "env", cfg.Env)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// healthHandler returns a health check response with dependency status.
func healthHandler(pool *pgxpool.Pool, rdb *cache.Redis) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status := map[string]string{
			"status":   "ok",
			"database": "ok",
			"redis":    "ok",
		}
		httpStatus := http.StatusOK

		if err := pool.Ping(ctx); err != nil {
			status["database"] = "error"
			status["status"] = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		if err := rdb.Ping(ctx); err != nil {
			status["redis"] = "error"
			status["status"] = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		json.NewEncoder(w).Encode(status)
	}
}

// Middleware chain.
type middleware func(http.Handler) http.Handler

func withMiddleware(h http.Handler, middlewares ...middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start).String(),
		)
	})
}

func corsMiddleware(cfg *config.Config) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", cfg.BaseURL)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
