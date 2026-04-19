CREATE TABLE IF NOT EXISTS announcements (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    idx_id TEXT NOT NULL unique,
    stock_code TEXT NOT NULL,
    announcement_title TEXT NOT NULL,
    date TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
