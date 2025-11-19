CREATE TABLE IF NOT EXISTS installed_extensions (
    extension_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    install_path TEXT NOT NULL,
    bundled BOOLEAN NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_installed_extensions_bundled
ON installed_extensions(bundled);

CREATE INDEX IF NOT EXISTS idx_installed_extensions_enabled
ON installed_extensions(enabled);

CREATE INDEX IF NOT EXISTS idx_installed_extensions_deleted
ON installed_extensions(deleted_at);
