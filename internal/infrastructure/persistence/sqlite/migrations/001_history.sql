-- +goose Up
-- History tracking for visited URLs

CREATE TABLE IF NOT EXISTS history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT,
    favicon_url TEXT,
    visit_count INTEGER DEFAULT 1,
    last_visited DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_history_url ON history(url);
CREATE INDEX IF NOT EXISTS idx_history_last_visited ON history(last_visited);
CREATE INDEX IF NOT EXISTS idx_history_visit_count_last_visited ON history(visit_count DESC, last_visited DESC);

-- FTS5 for history search
CREATE VIRTUAL TABLE IF NOT EXISTS history_fts USING fts5(
    url,
    title,
    content='history',
    content_rowid='id'
);

-- Triggers to keep FTS in sync
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

-- +goose Down
DROP TRIGGER IF EXISTS history_fts_update;
DROP TRIGGER IF EXISTS history_fts_delete;
DROP TRIGGER IF EXISTS history_fts_insert;
DROP TABLE IF EXISTS history_fts;
DROP TABLE IF EXISTS history;
