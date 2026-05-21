-- attachments tracks individual files associated with an announcement and their S3 status
CREATE TABLE IF NOT EXISTS attachments (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    idx_announcement_id TEXT NOT NULL REFERENCES announcements(idx_id) ON DELETE CASCADE,
    idx_url TEXT NOT NULL,
    error TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    filename TEXT NOT NULL,
    checksum TEXT NOT NULL,
    storage_path TEXT NOT NULL,
  title text not null,
    stock_code text NOT NULL,
    is_downloaded BOOLEAN NOT NULL,
    is_processing BOOLEAN NOT NULL DEFAULT false,
    date TIMESTAMPTZ NOT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_attachments_unprocessed
ON attachments(is_downloaded, is_processing, error)
WHERE is_downloaded = false AND is_processing = false AND error = '';
