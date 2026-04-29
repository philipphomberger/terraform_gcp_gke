CREATE TABLE files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id UUID REFERENCES files(id) ON DELETE CASCADE,
    name VARCHAR(512) NOT NULL,
    size BIGINT DEFAULT 0,
    mime_type VARCHAR(255),
    storage_key VARCHAR(1024),
    checksum VARCHAR(128),
    is_folder BOOLEAN DEFAULT FALSE,
    status VARCHAR(16) DEFAULT 'pending',  -- 'pending', 'ready', 'error'
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Soft-delete aware unique constraint: same user can't have two active files
-- with the same name in the same folder.
CREATE UNIQUE INDEX idx_files_unique_active
    ON files(user_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'), name)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_files_user_id ON files(user_id);
CREATE INDEX idx_files_parent_id ON files(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_files_checksum ON files(checksum) WHERE checksum IS NOT NULL;
CREATE INDEX idx_files_search ON files USING GIN (to_tsvector('english', name));
