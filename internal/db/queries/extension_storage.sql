-- name: GetExtensionStorageItem :one
SELECT value FROM extension_storage
WHERE extension_id = ? AND storage_type = ? AND key = ?
LIMIT 1;

-- name: GetAllExtensionStorageItems :many
SELECT key, value FROM extension_storage
WHERE extension_id = ? AND storage_type = ?;

-- name: UpsertExtensionStorageItem :exec
INSERT INTO extension_storage (extension_id, storage_type, key, value, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(extension_id, storage_type, key)
DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP;

-- name: DeleteExtensionStorageItem :exec
DELETE FROM extension_storage
WHERE extension_id = ? AND storage_type = ? AND key = ?;

-- name: ClearExtensionStorage :exec
DELETE FROM extension_storage
WHERE extension_id = ? AND storage_type = ?;

-- name: GetExtensionStorageKeys :many
SELECT key FROM extension_storage
WHERE extension_id = ? AND storage_type = ?;

-- name: CountExtensionStorageItems :one
SELECT COUNT(*) FROM extension_storage
WHERE extension_id = ? AND storage_type = ?;
