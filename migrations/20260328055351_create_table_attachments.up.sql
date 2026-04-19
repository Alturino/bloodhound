-- attachments tracks individual files associated with an announcement and their S3 status
CREATE TABLE IF NOT EXISTS attachments (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    idx_announcement_id TEXT NOT NULL REFERENCES announcements(idx_id) ON DELETE CASCADE,
    original_filename TEXT NOT NULL,
    checksum TEXT NOT NULL, -- SHA256 first 8 chars
    storage_path TEXT NOT NULL, -- S3 key
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP -- timestamptz when it's uploaded to s3
);
