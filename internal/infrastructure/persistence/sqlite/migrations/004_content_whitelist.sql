-- +goose Up
-- Content filter whitelist - domains that bypass content filtering

CREATE TABLE IF NOT EXISTS content_whitelist (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_content_whitelist_domain ON content_whitelist(domain);

-- +goose Down
DROP TABLE IF EXISTS content_whitelist;
