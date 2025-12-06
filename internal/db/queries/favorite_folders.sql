-- Favorite folders CRUD operations

-- name: GetAllFolders :many
-- Get all folders ordered by position
SELECT *
FROM favorite_folders
ORDER BY position ASC;

-- name: GetFolderByID :one
-- Get a specific folder by ID
SELECT *
FROM favorite_folders
WHERE id = ?
LIMIT 1;

-- name: CreateFolder :one
-- Insert a new folder with auto-incremented position
INSERT INTO favorite_folders (name, icon, position)
VALUES (
    ?,
    ?,
    COALESCE((SELECT MAX(position) + 1 FROM favorite_folders), 0)
)
RETURNING *;

-- name: UpdateFolder :exec
-- Update folder name and icon
UPDATE favorite_folders
SET
    name = ?,
    icon = ?
WHERE id = ?;

-- name: UpdateFolderPosition :exec
-- Update folder position
UPDATE favorite_folders
SET position = ?
WHERE id = ?;

-- name: DeleteFolder :exec
-- Delete a folder by ID (favorites will have folder_id set to NULL)
DELETE FROM favorite_folders
WHERE id = ?;

-- name: GetFolderCount :one
-- Get total count of folders
SELECT COUNT(*) as count
FROM favorite_folders;

-- name: GetFavoritesInFolder :many
-- Get all favorites in a specific folder
SELECT f.*
FROM favorites f
WHERE f.folder_id = ?
ORDER BY f.position ASC;

-- name: GetFavoritesWithoutFolder :many
-- Get all favorites not in any folder
SELECT *
FROM favorites
WHERE folder_id IS NULL
ORDER BY position ASC;
