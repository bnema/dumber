-- Remove shortcuts table
-- Shortcuts are now managed exclusively through configuration files
-- This table was redundant as shortcuts were synced from config on startup
-- but never queried during runtime (parser reads from config directly)

DROP TABLE IF EXISTS shortcuts;
