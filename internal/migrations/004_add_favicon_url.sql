-- Add favicon_url column to history table
-- Store favicon URLs for better UI display in dmenu

ALTER TABLE history ADD COLUMN favicon_url TEXT DEFAULT NULL;
CREATE INDEX IF NOT EXISTS idx_history_favicon ON history(favicon_url);