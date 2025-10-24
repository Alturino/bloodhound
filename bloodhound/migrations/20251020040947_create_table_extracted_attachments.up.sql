CREATE TABLE extracted_attachments (
    id uuid PRIMARY KEY,
    attachment_id uuid,
    name text,
    path text,
    created_at timestamptz,
    FOREIGN KEY (attachment_id) REFERENCES attachments (id)
);

CREATE INDEX IF NOT EXISTS idx_extracted_attachments_attachment_id ON extracted_attachments (
    attachment_id
);
CREATE INDEX IF NOT EXISTS idx_extracted_attachments_name ON extracted_attachments (
    name
);
CREATE INDEX IF NOT EXISTS idx_extracted_attachments_path ON extracted_attachments (
    path
);
CREATE INDEX IF NOT EXISTS idx_extracted_attachments_created_at ON extracted_attachments (
    created_at
);
