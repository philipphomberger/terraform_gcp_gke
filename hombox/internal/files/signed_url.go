package files

import (
	"context"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
)

// SignedUploadURL returns a pre-signed PUT URL for direct client-to-S3 upload.
func (s *Storage) SignedUploadURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	url, err := s.Client.PresignedPutObject(ctx, s.Bucket, key, ttl)
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return url.String(), nil
}

// SignedDownloadURL returns a pre-signed GET URL for direct download.
func (s *Storage) SignedDownloadURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	url, err := s.Client.PresignedGetObject(ctx, s.Bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return url.String(), nil
}

// HeadObject returns object metadata without downloading.
func (s *Storage) HeadObject(ctx context.Context, key string) (*minio.ObjectInfo, error) {
	info, err := s.Client.StatObject(ctx, s.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("stat object: %w", err)
	}
	return &info, nil
}

// GetObject downloads an object from S3 (for server-side processing like thumbnails).
func (s *Storage) GetObject(ctx context.Context, key string) (*minio.Object, error) {
	return s.Client.GetObject(ctx, s.Bucket, key, minio.GetObjectOptions{})
}
