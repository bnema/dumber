-- Add zoom persistence table
-- Store zoom levels per URL for restoration between sessions

CREATE TABLE IF NOT EXISTS zoom_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    zoom_level REAL NOT NULL DEFAULT 1.0,
    last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_zoom_settings_url ON zoom_settings(url);