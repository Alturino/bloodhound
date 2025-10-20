CREATE TYPE attachment_type AS ENUM (
    'public_expose',
    'corp_action',
    'earning_call',
    'pers',
    'shares_ownerships_update',
    'shares_outstanding_update',
    'others'
);

CREATE TABLE IF NOT EXISTS attachments (
    id uuid PRIMARY KEY,
    announcement_id uuid,
    name string,
    path string,
    source_url string,
    type attachment_type,
    created_at timestamptz,
    updated_at timestamptz,
    FOREIGN KEY (announcement_id) REFERENCES announcements (id)
);

CREATE INDEX IF NOT EXISTS idx_attachments_announcement_id ON attachments (
    announcement_id
);
CREATE INDEX IF NOT EXISTS idx_attachments_name ON attachments (name);
CREATE INDEX IF NOT EXISTS idx_attachments_path ON attachments (path);
CREATE INDEX IF NOT EXISTS idx_attachments_created_at ON attachments (
    created_at
);
