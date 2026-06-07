-- +goose Up
-- Store favicon cache metadata. Binary favicon data remains on disk.

CREATE TABLE IF NOT EXISTS favicons (
    key TEXT PRIMARY KEY,
    source_url TEXT,
    page_url TEXT,
    source TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    content_type TEXT,
    updated_at TIMESTAMP NOT NULL,
    last_checked_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS favicons;
