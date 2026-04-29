package files

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/philipphomberger/hombox/internal/config"
)

// Storage wraps MinIO client for file blob operations.
type Storage struct {
	Client     *minio.Client
	Bucket     string
	DerivBucket string
}

// Connect creates a MinIO client and ensures buckets exist.
func Connect(cfg config.S3Config) (*Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	ctx := context.Background()

	// Ensure buckets exist
	for _, bucket := range []string{cfg.Bucket, cfg.DerivativesBucket} {
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			return nil, fmt.Errorf("check bucket %s: %w", bucket, err)
		}
		if !exists {
			if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				return nil, fmt.Errorf("create bucket %s: %w", bucket, err)
			}
			slog.Info("created bucket", "name", bucket)
		}
	}

	slog.Info("connected to MinIO", "endpoint", cfg.Endpoint)
	return &Storage{
		Client:     client,
		Bucket:     cfg.Bucket,
		DerivBucket: cfg.DerivativesBucket,
	}, nil
}
