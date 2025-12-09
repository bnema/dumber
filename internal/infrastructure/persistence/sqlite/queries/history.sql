-- name: GetRecentHistory :many
SELECT * FROM history ORDER BY last_visited DESC LIMIT ? OFFSET ?;

-- name: GetHistoryByURL :one
SELECT * FROM history WHERE url = ? LIMIT 1;

-- name: UpsertHistory :exec
INSERT INTO history (url, title, favicon_url)
VALUES (?, ?, ?)
ON CONFLICT(url)
DO UPDATE SET
    visit_count = visit_count + 1,
    last_visited = CURRENT_TIMESTAMP,
    title = COALESCE(EXCLUDED.title, history.title),
    favicon_url = COALESCE(EXCLUDED.favicon_url, history.favicon_url);

-- name: IncrementVisitCount :exec
UPDATE history
SET visit_count = visit_count + 1, last_visited = CURRENT_TIMESTAMP
WHERE url = ?;

-- name: SearchHistory :many
SELECT * FROM history
WHERE url LIKE '%' || ? || '%' OR title LIKE '%' || ? || '%'
ORDER BY visit_count DESC, last_visited DESC
LIMIT ?;

-- name: DeleteHistoryByID :exec
DELETE FROM history WHERE id = ?;

-- name: DeleteHistoryOlderThan :exec
DELETE FROM history WHERE last_visited < ?;

-- name: DeleteAllHistory :exec
DELETE FROM history;
