CREATE TABLE extracted_attachments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attachment_id uuid NOT NULL,
    name text NOT NULL,
    path text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
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
