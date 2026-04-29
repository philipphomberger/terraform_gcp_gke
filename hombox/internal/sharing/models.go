package sharing

import (
	"encoding/json"
	"time"
)

// SharePermissions define what a share recipient can do.
type SharePermissions struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
}

// Share represents a sharing relationship.
type Share struct {
	ID          string           `json:"id"`
	OwnerID     string           `json:"owner_id"`
	FileID      string           `json:"file_id"`
	ShareType   string           `json:"share_type"` // "user" or "anonymous"
	RecipientID *string          `json:"recipient_id,omitempty"`
	Token       *string          `json:"token,omitempty"`
	Permissions SharePermissions `json:"permissions"`
	ExpiresAt   *time.Time       `json:"expires_at,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
}

// CreateShareRequest is the request to create a new share.
type CreateShareRequest struct {
	FileID      string           `json:"file_id"`
	ShareType   string           `json:"share_type"` // "user" or "anonymous"
	RecipientID string           `json:"recipient_id,omitempty"`
	Permissions SharePermissions `json:"permissions"`
	ExpiresIn   int              `json:"expires_in,omitempty"` // hours, 0 = never
}

// Scan implements sql.Scanner for SharePermissions (stored as JSONB).
func (p *SharePermissions) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	return json.Unmarshal(src.([]byte), p)
}
