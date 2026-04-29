CREATE TABLE shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    share_type VARCHAR(16) NOT NULL CHECK (share_type IN ('user', 'anonymous')),
    recipient_id UUID REFERENCES users(id),
    token VARCHAR(64) UNIQUE,
    permissions JSONB NOT NULL DEFAULT '{"read":true,"write":false}',
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_shares_owner_id ON shares(owner_id);
CREATE INDEX idx_shares_file_id ON shares(file_id);
CREATE INDEX idx_shares_token ON shares(token) WHERE token IS NOT NULL;
CREATE INDEX idx_shares_recipient ON shares(recipient_id) WHERE recipient_id IS NOT NULL;
