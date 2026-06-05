CREATE TABLE IF NOT EXISTS announcements (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    idx_id TEXT NOT NULL UNIQUE,
    stock_code TEXT NOT NULL,
    title TEXT NOT NULL,
    date TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);
