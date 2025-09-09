-- name: GetZoomLevel :one
-- Get zoom level for a specific domain
SELECT zoom_factor FROM zoom_levels WHERE domain = ? LIMIT 1;

-- name: SetZoomLevel :exec
-- Set or update zoom level for a domain with validation
INSERT INTO zoom_levels (domain, zoom_factor, updated_at) 
VALUES (?, ?, CURRENT_TIMESTAMP) 
ON CONFLICT(domain) DO UPDATE SET 
    zoom_factor = excluded.zoom_factor,
    updated_at = excluded.updated_at;

-- name: DeleteZoomLevel :exec
-- Delete zoom level setting for a domain
DELETE FROM zoom_levels WHERE domain = ?;

-- name: ListZoomLevels :many
-- List all zoom level settings ordered by most recently updated
SELECT domain, zoom_factor, updated_at FROM zoom_levels ORDER BY updated_at DESC;

-- name: CleanupOldZoomLevels :exec
-- Cleanup zoom level entries older than specified days
DELETE FROM zoom_levels WHERE updated_at < datetime('now', '-' || ? || ' days');

-- name: GetZoomLevelWithDefault :one
-- Get zoom level for domain with default fallback
SELECT COALESCE(
    (SELECT zoom_factor FROM zoom_levels WHERE domain = ? LIMIT 1),
    1.0
) as zoom_factor;