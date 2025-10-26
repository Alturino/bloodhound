CREATE TABLE IF NOT EXISTS analyzed_companies (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid NOT NULL,
    extracted_attachment_id uuid NOT NULL,
    path text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (company_id) REFERENCES companies (id),
    FOREIGN KEY (extracted_attachment_id) REFERENCES extracted_attachments (id)
);


CREATE INDEX IF NOT EXISTS idx_analyzed_companies_company_id ON analyzed_companies (
    company_id
);
CREATE INDEX IF NOT EXISTS idx_analyzed_companies_extracted_attachment_id ON analyzed_companies (
    extracted_attachment_id
);
CREATE INDEX IF NOT EXISTS idx_analyzed_companies_path ON analyzed_companies (
    path
);
CREATE INDEX IF NOT EXISTS idx_analyzed_companies_created_at ON analyzed_companies (
    created_at
);
