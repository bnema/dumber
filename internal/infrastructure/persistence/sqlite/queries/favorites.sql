-- name: GetAllFavorites :many
SELECT id, url, title, favicon_url, shortcut_key, position, created_at, updated_at
FROM favorites
ORDER BY position ASC;

-- name: GetFavoriteByID :one
SELECT id, url, title, favicon_url, shortcut_key, position, created_at, updated_at
FROM favorites
WHERE id = ? LIMIT 1;

-- name: GetFavoriteByURL :one
SELECT id, url, title, favicon_url, shortcut_key, position, created_at, updated_at
FROM favorites
WHERE url = ? LIMIT 1;

-- name: GetFavoritesByTag :many
SELECT f.id, f.url, f.title, f.favicon_url, f.shortcut_key, f.position, f.created_at, f.updated_at
FROM favorites f
INNER JOIN favorite_tag_assignments fta ON f.id = fta.favorite_id
WHERE fta.tag_id = ?
ORDER BY f.position ASC;

-- name: GetFavoriteByShortcut :one
SELECT id, url, title, favicon_url, shortcut_key, position, created_at, updated_at
FROM favorites
WHERE shortcut_key = ? LIMIT 1;

-- name: CreateFavorite :one
INSERT INTO favorites (url, title, favicon_url, position, created_at, updated_at)
VALUES (?, ?, ?, COALESCE((SELECT MAX(position) + 1 FROM favorites), 0), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING id, url, title, favicon_url, shortcut_key, position, created_at, updated_at;

-- name: UpdateFavorite :exec
UPDATE favorites
SET title = ?, favicon_url = ?, shortcut_key = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: UpdateFavoritePosition :exec
UPDATE favorites SET position = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: SetFavoriteShortcut :exec
UPDATE favorites SET shortcut_key = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteFavorite :exec
DELETE FROM favorites WHERE id = ?;
