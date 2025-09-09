-- Add zoom_levels table to replace zoom_settings with spec requirements
-- Store zoom levels per domain (not full URL) with Firefox zoom constraints

-- Drop existing zoom_settings table
DROP TABLE IF EXISTS zoom_settings;

-- Create new zoom_levels table following spec requirements
CREATE TABLE zoom_levels (
    domain TEXT PRIMARY KEY,
    zoom_factor REAL NOT NULL DEFAULT 1.0 CHECK(zoom_factor >= 0.3 AND zoom_factor <= 5.0),
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create index for performance on updated_at for cleanup operations
CREATE INDEX idx_zoom_levels_updated_at ON zoom_levels(updated_at);