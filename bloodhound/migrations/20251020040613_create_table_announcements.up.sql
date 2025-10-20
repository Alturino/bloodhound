CREATE TABLE IF NOT EXISTS announcements (
    id uuid PRIMARY KEY,
    company_id uuid,
    name string,
    created_at timestamptz,
    updated_at timestamptz,
    FOREIGN KEY (company_id) REFERENCES companies (id)
);

CREATE INDEX IF NOT EXISTS idx_announcements_company_id ON announcements (
    company_id
);
CREATE INDEX IF NOT EXISTS idx_announcements_name ON announcements (name);
CREATE INDEX IF NOT EXISTS idx_announcements_created_at ON announcements (
    created_at
);
