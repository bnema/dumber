-- +goose Up
-- Per-domain zoom levels with constraints

CREATE TABLE IF NOT EXISTS zoom_levels (
    domain TEXT PRIMARY KEY,
    zoom_factor REAL NOT NULL DEFAULT 1.0 CHECK(zoom_factor >= 0.25 AND zoom_factor <= 5.0),
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_zoom_levels_updated_at ON zoom_levels(updated_at);

-- +goose Down
DROP TABLE IF EXISTS zoom_levels;
