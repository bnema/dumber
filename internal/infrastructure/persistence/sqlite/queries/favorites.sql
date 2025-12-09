-- name: GetAllFavorites :many
SELECT * FROM favorites ORDER BY position ASC;

-- name: GetFavoriteByID :one
SELECT * FROM favorites WHERE id = ? LIMIT 1;

-- name: GetFavoriteByURL :one
SELECT * FROM favorites WHERE url = ? LIMIT 1;

-- name: GetFavoritesByFolder :many
SELECT * FROM favorites WHERE folder_id = ? ORDER BY position ASC;

-- name: GetFavoritesWithoutFolder :many
SELECT * FROM favorites WHERE folder_id IS NULL ORDER BY position ASC;

-- name: GetFavoriteByShortcut :one
SELECT * FROM favorites WHERE shortcut_key = ? LIMIT 1;

-- name: CreateFavorite :one
INSERT INTO favorites (url, title, favicon_url, folder_id, position, created_at, updated_at)
VALUES (?, ?, ?, ?, COALESCE((SELECT MAX(position) + 1 FROM favorites), 0), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: UpdateFavorite :exec
UPDATE favorites
SET title = ?, favicon_url = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: UpdateFavoritePosition :exec
UPDATE favorites SET position = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: SetFavoriteFolder :exec
UPDATE favorites SET folder_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: SetFavoriteShortcut :exec
UPDATE favorites SET shortcut_key = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteFavorite :exec
DELETE FROM favorites WHERE id = ?;
