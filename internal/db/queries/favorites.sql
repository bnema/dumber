-- name: GetAllFavorites :many
-- Get all favorites ordered by position
SELECT *
FROM favorites
ORDER BY position ASC;

-- name: GetFavoriteByURL :one
-- Get a specific favorite by URL
SELECT *
FROM favorites
WHERE url = ?
LIMIT 1;

-- name: CreateFavorite :exec
-- Insert a new favorite with auto-incremented position
INSERT INTO favorites (url, title, favicon_url, position, created_at, updated_at)
VALUES (
    ?,
    ?,
    ?,
    COALESCE((SELECT MAX(position) + 1 FROM favorites), 0),
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
);

-- name: UpdateFavorite :exec
-- Update favorite metadata (title, favicon)
UPDATE favorites
SET
    title = ?,
    favicon_url = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE url = ?;

-- name: DeleteFavorite :exec
-- Delete a favorite by URL
DELETE FROM favorites
WHERE url = ?;

-- name: IsFavorite :one
-- Check if a URL is favorited
SELECT COUNT(*) as count
FROM favorites
WHERE url = ?
LIMIT 1;

-- name: UpdateFavoritePosition :exec
-- Update the position of a favorite
UPDATE favorites
SET
    position = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE url = ?;

-- name: GetFavoriteCount :one
-- Get total count of favorites
SELECT COUNT(*) as count
FROM favorites;
