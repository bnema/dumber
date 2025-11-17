-- Add favorites table for omnibox favorites feature
-- Store user's favorited URLs with metadata for quick access

CREATE TABLE IF NOT EXISTS favorites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT,
    favicon_url TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create index for performance on position for ordered retrieval
CREATE INDEX IF NOT EXISTS idx_favorites_position ON favorites(position);

-- Create index for URL lookups
CREATE INDEX IF NOT EXISTS idx_favorites_url ON favorites(url);
