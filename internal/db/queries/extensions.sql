-- name: GetInstalledExtension :one
SELECT extension_id,
       name,
       version,
       install_path,
       bundled,
       enabled,
       created_at,
       updated_at
FROM installed_extensions
WHERE extension_id = ? AND deleted_at IS NULL
LIMIT 1;

-- name: ListInstalledExtensions :many
SELECT extension_id,
       name,
       version,
       install_path,
       bundled,
       enabled,
       created_at,
       updated_at
FROM installed_extensions
WHERE deleted_at IS NULL
ORDER BY bundled DESC, name ASC;

-- name: UpsertInstalledExtension :exec
INSERT INTO installed_extensions (extension_id, name, version, install_path, bundled, enabled, updated_at)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(extension_id) DO UPDATE SET
    name = excluded.name,
    version = excluded.version,
    install_path = excluded.install_path,
    bundled = excluded.bundled,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at,
    deleted_at = NULL;

-- name: SetExtensionEnabled :exec
UPDATE installed_extensions
SET enabled = ?, updated_at = CURRENT_TIMESTAMP
WHERE extension_id = ? AND deleted_at IS NULL;

-- name: MarkExtensionDeleted :exec
UPDATE installed_extensions
SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE extension_id = ?;

-- name: CheckExtensionNeedsUpdate :one
SELECT extension_id,
       CASE
           WHEN (julianday('now') - julianday(updated_at)) >= CAST(sqlc.arg(days) AS REAL) THEN 1
           ELSE 0
       END AS needs_update
FROM installed_extensions
WHERE extension_id = sqlc.arg(extension_id) AND deleted_at IS NULL
LIMIT 1;

-- name: CleanupOldDeletedExtensions :exec
DELETE FROM installed_extensions
WHERE deleted_at IS NOT NULL
  AND (julianday('now') - julianday(deleted_at)) >= CAST(sqlc.arg(days) AS REAL);
