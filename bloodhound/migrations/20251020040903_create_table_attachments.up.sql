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
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    announcement_id uuid NOT NULL,
    name text NOT NULL,
    path text NOT NULL,
    checksum text NOT NULL,
    source_url text NOT NULL,
    type attachment_type NOT NULL DEFAULT 'others',
    published_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (announcement_id) REFERENCES announcements (id)
);

CREATE INDEX IF NOT EXISTS idx_attachments_announcement_id ON attachments (
    announcement_id
);
CREATE INDEX IF NOT EXISTS idx_attachments_name ON attachments (name);
CREATE INDEX IF NOT EXISTS idx_attachments_path ON attachments (path);
CREATE INDEX IF NOT EXISTS idx_attachments_checksum ON attachments (checksum);
CREATE INDEX IF NOT EXISTS idx_attachments_created_at ON attachments (
    created_at
);
