package sharing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service handles share creation and access management.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates a new sharing service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Create creates a new share (user-targeted or anonymous link).
func (s *Service) Create(ctx context.Context, ownerID string, req CreateShareRequest) (*Share, error) {
	if req.ShareType != "user" && req.ShareType != "anonymous" {
		return nil, fmt.Errorf("share_type must be 'user' or 'anonymous'")
	}

	// Verify the file exists and belongs to the owner
	var isFolder bool
	err := s.pool.QueryRow(ctx,
		`SELECT is_folder FROM files WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		req.FileID, ownerID,
	).Scan(&isFolder)
	if err != nil {
		return nil, fmt.Errorf("file not found or not owned by you")
	}

	// Generate token for anonymous shares
	var token *string
	if req.ShareType == "anonymous" {
		t := generateShareToken()
		token = &t
	}

	// Compute expiration
	var expiresAt any
	if req.ExpiresIn > 0 {
		et := time.Now().Add(time.Duration(req.ExpiresIn) * time.Hour)
		expiresAt = et
	}

	// Set recipient for user shares
	var recipientID any
	if req.ShareType == "user" && req.RecipientID != "" {
		recipientID = req.RecipientID
	}

	var share Share
	err = s.pool.QueryRow(ctx,
		`INSERT INTO shares (owner_id, file_id, share_type, recipient_id, token, permissions, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, owner_id, file_id, share_type, recipient_id, token, permissions, expires_at, created_at`,
		ownerID, req.FileID, req.ShareType, recipientID, token, req.Permissions, expiresAt,
	).Scan(&share.ID, &share.OwnerID, &share.FileID, &share.ShareType,
		&share.RecipientID, &share.Token, &share.Permissions, &share.ExpiresAt, &share.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create share: %w", err)
	}

	return &share, nil
}

// ListByOwner returns all shares created by a user.
func (s *Service) ListByOwner(ctx context.Context, ownerID string) ([]Share, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, owner_id, file_id, share_type, recipient_id, token, permissions, expires_at, created_at
		 FROM shares WHERE owner_id = $1 ORDER BY created_at DESC`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list shares: %w", err)
	}
	defer rows.Close()

	var shares []Share
	for rows.Next() {
		var sh Share
		if err := rows.Scan(&sh.ID, &sh.OwnerID, &sh.FileID, &sh.ShareType,
			&sh.RecipientID, &sh.Token, &sh.Permissions, &sh.ExpiresAt, &sh.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan share: %w", err)
		}
		shares = append(shares, sh)
	}
	if shares == nil {
		shares = []Share{}
	}
	return shares, nil
}

// GetByToken retrieves a share by its public token.
func (s *Service) GetByToken(ctx context.Context, token string) (*Share, error) {
	var share Share
	err := s.pool.QueryRow(ctx,
		`SELECT id, owner_id, file_id, share_type, recipient_id, token, permissions, expires_at, created_at
		 FROM shares WHERE token = $1`,
		token,
	).Scan(&share.ID, &share.OwnerID, &share.FileID, &share.ShareType,
		&share.RecipientID, &share.Token, &share.Permissions, &share.ExpiresAt, &share.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("share not found")
	}

	// Check expiration
	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		return nil, fmt.Errorf("share has expired")
	}

	return &share, nil
}

// Delete removes a share (only the owner can delete).
func (s *Service) Delete(ctx context.Context, ownerID, shareID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM shares WHERE id = $1 AND owner_id = $2`, shareID, ownerID)
	return err
}

// GetSharedFile returns file info for a shared folder (for listing shared contents).
func (s *Service) GetSharedFile(ctx context.Context, shareToken string, parentID *string) ([]SharedFile, error) {
	// First, get the share and check permissions
	share, err := s.GetByToken(ctx, shareToken)
	if err != nil {
		return nil, err
	}

	if !share.Permissions.Read {
		return nil, fmt.Errorf("no read permission on this share")
	}

	// Determine which folder to list
	targetFolderID := share.FileID // root of shared folder
	if parentID != nil && *parentID != "" {
		targetFolderID = *parentID
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, parent_id, name, size, mime_type, is_folder, status, created_at, updated_at
		 FROM files WHERE user_id = $1 AND deleted_at IS NULL AND status = 'ready'
		 AND parent_id = $2
		 ORDER BY is_folder DESC, name ASC`,
		share.OwnerID, targetFolderID,
	)
	if err != nil {
		return nil, fmt.Errorf("list shared files: %w", err)
	}
	defer rows.Close()

	var files []SharedFile
	for rows.Next() {
		var f SharedFile
		if err := rows.Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.Size,
			&f.MimeType, &f.IsFolder, &f.Status, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, f)
	}
	if files == nil {
		files = []SharedFile{}
	}
	return files, nil
}

// SharedFile is a file accessible via a share link.
type SharedFile struct {
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

func generateShareToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
