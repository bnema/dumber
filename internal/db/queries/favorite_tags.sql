-- Favorite tags CRUD and assignment operations

-- name: GetAllTags :many
SELECT * FROM favorite_tags ORDER BY name ASC;

-- name: GetTagByID :one
SELECT * FROM favorite_tags WHERE id = ? LIMIT 1;

-- name: GetTagByName :one
SELECT * FROM favorite_tags WHERE name = ? LIMIT 1;

-- name: CreateTag :one
INSERT INTO favorite_tags (name, color) VALUES (?, ?) RETURNING *;

-- name: UpdateTag :exec
UPDATE favorite_tags SET name = ?, color = ? WHERE id = ?;

-- name: DeleteTag :exec
DELETE FROM favorite_tags WHERE id = ?;

-- name: GetTagCount :one
SELECT COUNT(*) as count FROM favorite_tags;

-- name: AssignTag :exec
INSERT OR IGNORE INTO favorite_tag_assignments (favorite_id, tag_id) VALUES (?, ?);

-- name: RemoveTag :exec
DELETE FROM favorite_tag_assignments WHERE favorite_id = ? AND tag_id = ?;

-- name: GetTagsForFavorite :many
SELECT t.* FROM favorite_tags t INNER JOIN favorite_tag_assignments fta ON t.id = fta.tag_id WHERE fta.favorite_id = ? ORDER BY t.name ASC;

-- name: GetFavoritesWithTag :many
SELECT f.* FROM favorites f INNER JOIN favorite_tag_assignments fta ON f.id = fta.favorite_id WHERE fta.tag_id = ? ORDER BY f.position ASC;

-- name: ClearTagsFromFavorite :exec
DELETE FROM favorite_tag_assignments WHERE favorite_id = ?;

-- name: GetTagUsageCount :one
SELECT COUNT(*) as count FROM favorite_tag_assignments WHERE tag_id = ?;
