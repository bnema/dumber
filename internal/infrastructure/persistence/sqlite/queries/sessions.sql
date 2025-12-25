-- name: InsertSession :exec
INSERT INTO sessions (id, type, started_at, ended_at)
VALUES (?, ?, ?, ?);

-- name: GetSessionByID :one
SELECT * FROM sessions WHERE id = ? LIMIT 1;

-- name: GetActiveBrowserSession :one
SELECT * FROM sessions
WHERE type = 'browser' AND ended_at IS NULL
ORDER BY started_at DESC
LIMIT 1;

-- name: GetRecentSessions :many
SELECT * FROM sessions
ORDER BY started_at DESC
LIMIT ?;

-- name: MarkSessionEnded :exec
UPDATE sessions
SET ended_at = ?
WHERE id = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = ?;
