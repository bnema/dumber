-- +goose Up
-- Favorites (bookmarks) with folders, tags, and shortcuts

-- Folders for organizing favorites
CREATE TABLE IF NOT EXISTS favorite_folders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    icon TEXT,
    parent_id INTEGER REFERENCES favorite_folders(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_favorite_folders_position ON favorite_folders(position);
CREATE INDEX IF NOT EXISTS idx_favorite_folders_parent ON favorite_folders(parent_id);

-- Tags for labeling favorites
CREATE TABLE IF NOT EXISTS favorite_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    color TEXT NOT NULL DEFAULT '#808080',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Favorites table
CREATE TABLE IF NOT EXISTS favorites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT,
    favicon_url TEXT,
    folder_id INTEGER REFERENCES favorite_folders(id) ON DELETE SET NULL,
    shortcut_key INTEGER CHECK (shortcut_key BETWEEN 1 AND 9),
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_favorites_position ON favorites(position);
CREATE INDEX IF NOT EXISTS idx_favorites_url ON favorites(url);
CREATE INDEX IF NOT EXISTS idx_favorites_folder ON favorites(folder_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_favorites_shortcut ON favorites(shortcut_key) WHERE shortcut_key IS NOT NULL;

-- Tag assignments (junction table)
CREATE TABLE IF NOT EXISTS favorite_tag_assignments (
    favorite_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (favorite_id, tag_id),
    FOREIGN KEY (favorite_id) REFERENCES favorites(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES favorite_tags(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_fta_favorite ON favorite_tag_assignments(favorite_id);
CREATE INDEX IF NOT EXISTS idx_fta_tag ON favorite_tag_assignments(tag_id);

-- +goose Down
DROP TABLE IF EXISTS favorite_tag_assignments;
DROP TABLE IF EXISTS favorites;
DROP TABLE IF EXISTS favorite_tags;
DROP TABLE IF EXISTS favorite_folders;
