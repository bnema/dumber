-- name: GetAllFolders :many
SELECT * FROM favorite_folders ORDER BY position ASC;

-- name: GetFolderByID :one
SELECT * FROM favorite_folders WHERE id = ? LIMIT 1;

-- name: GetChildFolders :many
SELECT * FROM favorite_folders WHERE parent_id = ? ORDER BY position ASC;

-- name: GetRootFolders :many
SELECT * FROM favorite_folders WHERE parent_id IS NULL ORDER BY position ASC;

-- name: CreateFolder :one
INSERT INTO favorite_folders (name, icon, parent_id, position, created_at)
VALUES (?, ?, ?, COALESCE((SELECT MAX(position) + 1 FROM favorite_folders), 0), CURRENT_TIMESTAMP)
RETURNING *;

-- name: UpdateFolder :exec
UPDATE favorite_folders SET name = ?, icon = ? WHERE id = ?;

-- name: UpdateFolderPosition :exec
UPDATE favorite_folders SET position = ? WHERE id = ?;

-- name: DeleteFolder :exec
DELETE FROM favorite_folders WHERE id = ?;
