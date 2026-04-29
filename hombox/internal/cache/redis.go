package cache

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/philipphomberger/hombox/internal/config"
)

// Redis wraps the go-redis client with helpers.
type Redis struct {
	Client *redis.Client
}

// Connect establishes a connection to Redis.
func Connect(cfg config.RedisConfig) (*Redis, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	slog.Info("connected to Redis")
	return &Redis{Client: client}, nil
}

// Close shuts down the Redis connection pool.
func (r *Redis) Close() error {
	return r.Client.Close()
}

// SetSession stores a session token with an expiry.
func (r *Redis) SetSession(ctx context.Context, token string, userID string, ttl time.Duration) error {
	return r.Client.Set(ctx, "session:"+token, userID, ttl).Err()
}

// GetSession retrieves a user ID by session token. Returns empty string if not found.
func (r *Redis) GetSession(ctx context.Context, token string) (string, error) {
	val, err := r.Client.Get(ctx, "session:"+token).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// DeleteSession removes a session token.
func (r *Redis) DeleteSession(ctx context.Context, token string) error {
	return r.Client.Del(ctx, "session:"+token).Err()
}

// Ping checks Redis connectivity.
func (r *Redis) Ping(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}
