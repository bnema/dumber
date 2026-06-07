-- name: GetFavicon :one
SELECT * FROM favicons WHERE key = ? LIMIT 1;

-- name: UpsertFavicon :exec
INSERT INTO favicons (
    key,
    source_url,
    page_url,
    source,
    content_hash,
    content_type,
    updated_at,
    last_checked_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
    source_url = EXCLUDED.source_url,
    page_url = EXCLUDED.page_url,
    source = EXCLUDED.source,
    content_hash = EXCLUDED.content_hash,
    content_type = EXCLUDED.content_type,
    updated_at = EXCLUDED.updated_at,
    last_checked_at = EXCLUDED.last_checked_at;

-- name: UpdateFaviconLastChecked :exec
UPDATE favicons
SET content_hash = ?, last_checked_at = ?
WHERE key = ?;

-- name: DeleteFavicon :exec
DELETE FROM favicons WHERE key = ?;
