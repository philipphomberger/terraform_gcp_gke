package files

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"golang.org/x/image/draw"

	"github.com/philipphomberger/hombox/internal/config"
)

// Service manages file metadata and coordinates storage operations.
type Service struct {
	pool    *pgxpool.Pool
	storage *Storage
	cfg     config.UploadConfig
}

// File represents a file or folder entry.
type File struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ParentID  *string   `json:"parent_id,omitempty"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	MimeType  *string   `json:"mime_type,omitempty"`
	IsFolder  bool      `json:"is_folder"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewService creates a new file service.
func NewService(pool *pgxpool.Pool, storage *Storage, cfg config.UploadConfig) *Service {
	return &Service{pool: pool, storage: storage, cfg: cfg}
}

// InitiateUpload creates a pending file record and returns a signed upload URL.
func (s *Service) InitiateUpload(ctx context.Context, userID string, parentID *string, name string, size int64, mimeType string) (*File, string, error) {
	id := uuid.New().String()
	storageKey := fmt.Sprintf("files/%s/%s", userID, id)

	var file File
	err := s.pool.QueryRow(ctx,
		`INSERT INTO files (id, user_id, parent_id, name, size, mime_type, storage_key, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		 RETURNING id, user_id, parent_id, name, size, mime_type, is_folder, status, created_at, updated_at`,
		id, userID, parentID, name, size, mimeType, storageKey,
	).Scan(&file.ID, &file.UserID, &file.ParentID, &file.Name, &file.Size,
		&file.MimeType, &file.IsFolder, &file.Status, &file.CreatedAt, &file.UpdatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("create file record: %w", err)
	}

	signedURL, err := s.storage.SignedUploadURL(ctx, storageKey, 15*time.Minute)
	if err != nil {
		return nil, "", fmt.Errorf("generate signed url: %w", err)
	}

	return &file, signedURL, nil
}

// ConfirmUpload marks a pending file as ready after client confirms upload.
func (s *Service) ConfirmUpload(ctx context.Context, userID, fileID, checksum string) (*File, error) {
	var storageKey string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_key FROM files WHERE id = $1 AND user_id = $2 AND status = 'pending'`,
		fileID, userID,
	).Scan(&storageKey)
	if err != nil {
		return nil, fmt.Errorf("file not found or not pending")
	}

	info, err := s.storage.HeadObject(ctx, storageKey)
	if err != nil {
		return nil, fmt.Errorf("object not found in storage: %w", err)
	}

	// Deduplication: check for existing file with same checksum
	if checksum != "" {
		var existingKey string
		_ = s.pool.QueryRow(ctx,
			`SELECT storage_key FROM files WHERE checksum = $1 AND user_id = $2 AND id != $3 AND deleted_at IS NULL LIMIT 1`,
			checksum, userID, fileID,
		).Scan(&existingKey)
		if existingKey != "" {
			_ = s.storage.Client.RemoveObject(ctx, s.storage.Bucket, storageKey, minio.RemoveObjectOptions{})
			storageKey = existingKey
		}
	}

	var file File
	err = s.pool.QueryRow(ctx,
		`UPDATE files SET status = 'ready', size = $3, checksum = $4, storage_key = $5, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2
		 RETURNING id, user_id, parent_id, name, size, mime_type, is_folder, status, created_at, updated_at`,
		fileID, userID, info.Size, checksum, storageKey,
	).Scan(&file.ID, &file.UserID, &file.ParentID, &file.Name, &file.Size,
		&file.MimeType, &file.IsFolder, &file.Status, &file.CreatedAt, &file.UpdatedAt)

	return &file, err
}

// GetDownloadURL returns a pre-signed GET URL for downloading a file.
func (s *Service) GetDownloadURL(ctx context.Context, userID, fileID string) (string, *File, error) {
	var file File
	var storageKey *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, parent_id, name, size, mime_type, storage_key, is_folder, status, created_at, updated_at
		 FROM files WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL AND status = 'ready'`,
		fileID, userID,
	).Scan(&file.ID, &file.UserID, &file.ParentID, &file.Name, &file.Size,
		&file.MimeType, &storageKey, &file.IsFolder, &file.Status, &file.CreatedAt, &file.UpdatedAt)
	if err != nil {
		return "", nil, fmt.Errorf("file not found")
	}

	if storageKey == nil {
		return "", nil, fmt.Errorf("file has no storage key")
	}

	url, err := s.storage.SignedDownloadURL(ctx, *storageKey, 5*time.Minute)
	return url, &file, err
}

// CreateFolder creates a new folder.
func (s *Service) CreateFolder(ctx context.Context, userID string, parentID *string, name string) (*File, error) {
	id := uuid.New().String()
	var file File
	err := s.pool.QueryRow(ctx,
		`INSERT INTO files (id, user_id, parent_id, name, is_folder, status)
		 VALUES ($1, $2, $3, $4, true, 'ready')
		 RETURNING id, user_id, parent_id, name, size, mime_type, is_folder, status, created_at, updated_at`,
		id, userID, parentID, name,
	).Scan(&file.ID, &file.UserID, &file.ParentID, &file.Name, &file.Size,
		&file.MimeType, &file.IsFolder, &file.Status, &file.CreatedAt, &file.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}
	return &file, nil
}

// List returns files and folders in a given parent.
func (s *Service) List(ctx context.Context, userID string, parentID *string) ([]File, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, parent_id, name, size, mime_type, is_folder, status, created_at, updated_at
		 FROM files WHERE user_id = $1 AND deleted_at IS NULL AND status = 'ready'
		 AND parent_id IS NOT DISTINCT FROM $2
		 ORDER BY is_folder DESC, name ASC`,
		userID, parentID,
	)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	var files []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.Size,
			&f.MimeType, &f.IsFolder, &f.Status, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, f)
	}
	if files == nil {
		files = []File{}
	}
	return files, nil
}

// Rename updates a file's name.
func (s *Service) Rename(ctx context.Context, userID, fileID, newName string) (*File, error) {
	var file File
	err := s.pool.QueryRow(ctx,
		`UPDATE files SET name = $3, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		 RETURNING id, user_id, parent_id, name, size, mime_type, is_folder, status, created_at, updated_at`,
		fileID, userID, newName,
	).Scan(&file.ID, &file.UserID, &file.ParentID, &file.Name, &file.Size,
		&file.MimeType, &file.IsFolder, &file.Status, &file.CreatedAt, &file.UpdatedAt)
	return &file, err
}

// Move changes a file's parent folder.
func (s *Service) Move(ctx context.Context, userID, fileID string, newParentID *string) (*File, error) {
	var file File
	err := s.pool.QueryRow(ctx,
		`UPDATE files SET parent_id = $3, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		 RETURNING id, user_id, parent_id, name, size, mime_type, is_folder, status, created_at, updated_at`,
		fileID, userID, newParentID,
	).Scan(&file.ID, &file.UserID, &file.ParentID, &file.Name, &file.Size,
		&file.MimeType, &file.IsFolder, &file.Status, &file.CreatedAt, &file.UpdatedAt)
	return &file, err
}

// Delete soft-deletes a file or folder.
func (s *Service) Delete(ctx context.Context, userID, fileID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE files SET deleted_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		fileID, userID)
	return err
}

// Search performs a full-text search on file names.
func (s *Service) Search(ctx context.Context, userID, query string) ([]File, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, parent_id, name, size, mime_type, is_folder, status, created_at, updated_at
		 FROM files WHERE user_id = $1 AND deleted_at IS NULL AND status = 'ready'
		 AND to_tsvector('english', name) @@ plainto_tsquery('english', $2)
		 ORDER BY is_folder DESC, name ASC LIMIT 50`,
		userID, query,
	)
	if err != nil {
		return nil, fmt.Errorf("search files: %w", err)
	}
	defer rows.Close()

	var files []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.Size,
			&f.MimeType, &f.IsFolder, &f.Status, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, f)
	}
	if files == nil {
		files = []File{}
	}
	return files, nil
}

// GeneratePreview creates a thumbnail for an image and returns a signed URL.
func (s *Service) GeneratePreview(ctx context.Context, userID, fileID string, width, height int) (string, error) {
	var storageKey string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_key FROM files WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		fileID, userID,
	).Scan(&storageKey)
	if err != nil {
		return "", fmt.Errorf("file not found")
	}

	derivKey := fmt.Sprintf("derivatives/%s_%dx%d.jpg", fileID, width, height)

	// Check if thumbnail already exists
	_, err = s.storage.Client.StatObject(ctx, s.storage.DerivBucket, derivKey, minio.StatObjectOptions{})
	if err == nil {
		url, _ := s.storage.SignedDownloadURL(ctx, derivKey, 5*time.Minute)
		return url, nil
	}

	// Download original
	obj, err := s.storage.GetObject(ctx, storageKey)
	if err != nil {
		return "", fmt.Errorf("get object: %w", err)
	}
	defer obj.Close()

	// Decode image
	img, _, err := image.Decode(obj)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	// Resize
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	// Encode to JPEG and upload
	pr, pw := io.Pipe()
	go func() {
		_ = jpeg.Encode(pw, dst, &jpeg.Options{Quality: 80})
		pw.Close()
	}()

	_, err = s.storage.Client.PutObject(ctx, s.storage.DerivBucket, derivKey, pr, -1, minio.PutObjectOptions{
		ContentType: "image/jpeg",
	})
	if err != nil {
		return "", fmt.Errorf("upload derivative: %w", err)
	}

	slog.Info("generated preview", "file_id", fileID, "derivative", derivKey)
	return s.storage.SignedDownloadURL(ctx, derivKey, 5*time.Minute)
}
