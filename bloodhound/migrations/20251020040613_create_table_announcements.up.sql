CREATE TABLE IF NOT EXISTS announcements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL,
    name text NOT NULL,
    published_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id) REFERENCES companies (id)
);

CREATE INDEX IF NOT EXISTS idx_announcements_company_id ON announcements (
    company_id
);
CREATE INDEX IF NOT EXISTS idx_announcements_name ON announcements (name);
CREATE INDEX IF NOT EXISTS idx_announcements_created_at ON announcements (
    created_at
);
