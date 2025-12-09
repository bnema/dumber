-- name: GetZoomLevel :one
SELECT * FROM zoom_levels WHERE domain = ? LIMIT 1;

-- name: SetZoomLevel :exec
INSERT INTO zoom_levels (domain, zoom_factor, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(domain) DO UPDATE SET
    zoom_factor = excluded.zoom_factor,
    updated_at = CURRENT_TIMESTAMP;

-- name: DeleteZoomLevel :exec
DELETE FROM zoom_levels WHERE domain = ?;

-- name: ListZoomLevels :many
SELECT * FROM zoom_levels ORDER BY updated_at DESC;
