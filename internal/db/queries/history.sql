-- name: GetHistory :many
SELECT id, url, title, visit_count, last_visited, created_at
FROM history
ORDER BY last_visited DESC
LIMIT ?;

-- name: AddOrUpdateHistory :exec
INSERT INTO history (url, title) 
VALUES (?, ?)
ON CONFLICT(url) 
DO UPDATE SET 
    visit_count = visit_count + 1,
    last_visited = CURRENT_TIMESTAMP,
    title = EXCLUDED.title;

-- name: SearchHistory :many
SELECT id, url, title, visit_count, last_visited, created_at
FROM history 
WHERE url LIKE '%' || ? || '%' OR title LIKE '%' || ? || '%'
ORDER BY visit_count DESC, last_visited DESC
LIMIT ?;

-- name: GetShortcuts :many
SELECT id, shortcut, url_template, description, created_at
FROM shortcuts
ORDER BY shortcut;