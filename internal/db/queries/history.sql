-- name: GetHistory :many
SELECT *
FROM history
ORDER BY last_visited DESC
LIMIT ?;

-- name: GetHistoryWithOffset :many
SELECT *
FROM history
ORDER BY last_visited DESC
LIMIT ? OFFSET ?;

-- name: AddOrUpdateHistory :exec
INSERT INTO history (url, title) 
VALUES (?, ?)
ON CONFLICT(url) 
DO UPDATE SET 
    visit_count = visit_count + 1,
    last_visited = CURRENT_TIMESTAMP,
    title = EXCLUDED.title;

-- name: SearchHistory :many
SELECT *
FROM history 
WHERE url LIKE '%' || ? || '%' OR title LIKE '%' || ? || '%'
ORDER BY visit_count DESC, last_visited DESC
LIMIT ?;

-- name: UpdateHistoryFavicon :exec
UPDATE history
SET favicon_url = ?
WHERE url = ?;

-- name: GetHistoryEntry :one
SELECT *
FROM history
WHERE url = ?
LIMIT 1;

-- name: DeleteHistory :exec
DELETE FROM history
WHERE id = ?;

-- name: GetShortcuts :many
SELECT id, shortcut, url_template, description, created_at
FROM shortcuts
ORDER BY shortcut;