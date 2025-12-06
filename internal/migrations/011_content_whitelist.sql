-- Add content_whitelist table for content blocker whitelist
-- Store user's whitelisted domains that bypass content filtering

CREATE TABLE IF NOT EXISTS content_whitelist (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create index for domain lookups
CREATE INDEX IF NOT EXISTS idx_content_whitelist_domain ON content_whitelist(domain);
