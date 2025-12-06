-- Homepage redesign: folders, tags, shortcuts, FTS5 for history

-- ═══════════════════════════════════════════════════════════════
-- FAVORITE FOLDERS
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS favorite_folders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    icon TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_favorite_folders_position ON favorite_folders(position);

-- ═══════════════════════════════════════════════════════════════
-- FAVORITE TAGS
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS favorite_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    color TEXT NOT NULL DEFAULT '#6b7280',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ═══════════════════════════════════════════════════════════════
-- TAG ASSIGNMENTS (junction table)
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS favorite_tag_assignments (
    favorite_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (favorite_id, tag_id),
    FOREIGN KEY (favorite_id) REFERENCES favorites(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES favorite_tags(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_fta_favorite ON favorite_tag_assignments(favorite_id);
CREATE INDEX IF NOT EXISTS idx_fta_tag ON favorite_tag_assignments(tag_id);

-- ═══════════════════════════════════════════════════════════════
-- EXTEND FAVORITES TABLE
-- ═══════════════════════════════════════════════════════════════
ALTER TABLE favorites ADD COLUMN folder_id INTEGER REFERENCES favorite_folders(id) ON DELETE SET NULL;
ALTER TABLE favorites ADD COLUMN shortcut_key INTEGER CHECK (shortcut_key BETWEEN 1 AND 9);

CREATE INDEX IF NOT EXISTS idx_favorites_folder ON favorites(folder_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_favorites_shortcut ON favorites(shortcut_key) WHERE shortcut_key IS NOT NULL;

-- ═══════════════════════════════════════════════════════════════
-- FTS5 FOR HISTORY SEARCH
-- ═══════════════════════════════════════════════════════════════
CREATE VIRTUAL TABLE IF NOT EXISTS history_fts USING fts5(
    url,
    title,
    content='history',
    content_rowid='id'
);

-- Triggers to keep FTS in sync with history table
CREATE TRIGGER IF NOT EXISTS history_fts_insert AFTER INSERT ON history BEGIN
    INSERT INTO history_fts(rowid, url, title) VALUES (NEW.id, NEW.url, NEW.title);
END;

CREATE TRIGGER IF NOT EXISTS history_fts_delete AFTER DELETE ON history BEGIN
    INSERT INTO history_fts(history_fts, rowid, url, title) VALUES('delete', OLD.id, OLD.url, OLD.title);
END;

CREATE TRIGGER IF NOT EXISTS history_fts_update AFTER UPDATE ON history BEGIN
    INSERT INTO history_fts(history_fts, rowid, url, title) VALUES('delete', OLD.id, OLD.url, OLD.title);
    INSERT INTO history_fts(rowid, url, title) VALUES (NEW.id, NEW.url, NEW.title);
END;

-- Populate FTS with existing history data
INSERT INTO history_fts(rowid, url, title) SELECT id, url, title FROM history;
