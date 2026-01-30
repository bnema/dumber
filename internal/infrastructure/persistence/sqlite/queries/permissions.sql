-- name: GetPermission :one
SELECT * FROM permissions WHERE origin = ? AND permission_type = ? LIMIT 1;

-- name: SetPermission :exec
INSERT INTO permissions (origin, permission_type, decision, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(origin, permission_type) DO UPDATE SET
    decision = excluded.decision,
    updated_at = CURRENT_TIMESTAMP;

-- name: DeletePermission :exec
DELETE FROM permissions WHERE origin = ? AND permission_type = ?;

-- name: ListPermissionsByOrigin :many
SELECT * FROM permissions WHERE origin = ? ORDER BY updated_at DESC;

-- name: ListAllPermissions :many
SELECT * FROM permissions ORDER BY origin, permission_type;
