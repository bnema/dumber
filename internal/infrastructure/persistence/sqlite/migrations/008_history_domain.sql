-- +goose Up
-- Store normalized history domains for indexed systemview filtering.

ALTER TABLE history ADD COLUMN domain TEXT;

UPDATE history
SET domain = LOWER(
    CASE
        WHEN INSTR(REPLACE(REPLACE(SUBSTR(url, INSTR(url, '://') + 3), '?', '/'), '#', '/'), '/') > 0 THEN
            SUBSTR(
                REPLACE(REPLACE(SUBSTR(url, INSTR(url, '://') + 3), '?', '/'), '#', '/'),
                1,
                INSTR(REPLACE(REPLACE(SUBSTR(url, INSTR(url, '://') + 3), '?', '/'), '#', '/'), '/') - 1
            )
        ELSE REPLACE(REPLACE(SUBSTR(url, INSTR(url, '://') + 3), '?', '/'), '#', '/')
    END
)
WHERE INSTR(url, '://') > 0;

-- Mirror domainurl.CanonicalDomain for legacy rows as closely as SQLite can:
-- strip userinfo first, then strip the leading www. alias.
UPDATE history
SET domain = SUBSTR(domain, INSTR(domain, '@') + 1)
WHERE INSTR(domain, '@') > 0;

UPDATE history
SET domain = SUBSTR(domain, 5)
WHERE domain LIKE 'www.%';

CREATE INDEX IF NOT EXISTS idx_history_domain_last_visited ON history(domain, last_visited DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_history_domain_last_visited;

-- Rebuild instead of ALTER TABLE DROP COLUMN so rollback works on SQLite
-- versions before 3.35. Preserve the base history schema and indexes/triggers.
DROP TRIGGER IF EXISTS history_fts_update;
DROP TRIGGER IF EXISTS history_fts_delete;
DROP TRIGGER IF EXISTS history_fts_insert;
DROP TABLE IF EXISTS history_fts;

CREATE TABLE history_008_down (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT,
    favicon_url TEXT,
    visit_count INTEGER DEFAULT 1,
    last_visited DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO history_008_down (id, url, title, favicon_url, visit_count, last_visited, created_at)
SELECT id, url, title, favicon_url, visit_count, last_visited, created_at
FROM history;

DROP TABLE history;
ALTER TABLE history_008_down RENAME TO history;

CREATE INDEX IF NOT EXISTS idx_history_url ON history(url);
CREATE INDEX IF NOT EXISTS idx_history_last_visited ON history(last_visited);
CREATE INDEX IF NOT EXISTS idx_history_visit_count_last_visited ON history(visit_count DESC, last_visited DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS history_fts USING fts5(
    url,
    title,
    content='history',
    content_rowid='id'
);

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS history_fts_insert AFTER INSERT ON history BEGIN
    INSERT INTO history_fts(rowid, url, title) VALUES (NEW.id, NEW.url, NEW.title);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS history_fts_delete AFTER DELETE ON history BEGIN
    INSERT INTO history_fts(history_fts, rowid, url, title) VALUES('delete', OLD.id, OLD.url, OLD.title);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS history_fts_update AFTER UPDATE ON history BEGIN
    INSERT INTO history_fts(history_fts, rowid, url, title) VALUES('delete', OLD.id, OLD.url, OLD.title);
    INSERT INTO history_fts(rowid, url, title) VALUES (NEW.id, NEW.url, NEW.title);
END;
-- +goose StatementEnd

INSERT INTO history_fts(rowid, url, title)
SELECT id, url, title FROM history;
