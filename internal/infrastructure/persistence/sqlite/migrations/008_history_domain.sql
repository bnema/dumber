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
ALTER TABLE history DROP COLUMN domain;
