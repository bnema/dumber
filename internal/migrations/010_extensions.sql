-- Extension storage for WebExtensions API
-- Stores key-value pairs for chrome.storage.local API

CREATE TABLE IF NOT EXISTS extension_storage (
    extension_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (extension_id, key)
);

CREATE INDEX IF NOT EXISTS idx_extension_storage_ext_id
ON extension_storage(extension_id);
