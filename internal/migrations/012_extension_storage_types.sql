-- Add storage_type column to distinguish between chrome.storage.local and localStorage
-- SQLite doesn't support ALTER PRIMARY KEY, so we recreate the table
-- This migration handles both fresh databases and upgrades from 010

-- Drop the temp table if it exists from a failed migration
DROP TABLE IF EXISTS extension_storage_new;

-- Create the new table structure
CREATE TABLE extension_storage_new (
    extension_id TEXT NOT NULL,
    storage_type TEXT NOT NULL DEFAULT 'storage.local',
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (extension_id, storage_type, key)
);

-- Migrate existing data if old table exists (upgrade path for existing users)
-- Uses INSERT SELECT which is a no-op if source table is empty or doesn't exist rows
INSERT INTO extension_storage_new (extension_id, storage_type, key, value, created_at, updated_at)
SELECT extension_id, 'storage.local', key, value, created_at, updated_at
FROM extension_storage;

-- Drop old table and rename new one
DROP TABLE extension_storage;
ALTER TABLE extension_storage_new RENAME TO extension_storage;

-- Create index for efficient lookups
CREATE INDEX IF NOT EXISTS idx_extension_storage_lookup
ON extension_storage(extension_id, storage_type);
