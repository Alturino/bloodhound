CREATE TABLE IF NOT EXISTS extracted_attachments (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    attachment_id UUID NOT NULL REFERENCES attachments (id) ON DELETE CASCADE,
    idx_announcement_id TEXT NOT NULL REFERENCES announcements (
        idx_id
    ) ON DELETE CASCADE,
    title TEXT NOT NULL,
    stock_code TEXT NOT NULL,
    extracted_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    minio_key TEXT NOT NULL,
    metadata JSONB DEFAULT '{}'::JSONB
);
