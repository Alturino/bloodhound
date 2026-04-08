CREATE TABLE IF NOT EXISTS announcements (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    idx_id TEXT UNIQUE NOT NULL,
    stock_code VARCHAR(4) NOT NULL,
    announcement_title TEXT NOT NULL,
    announcement_date TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
