-- name: GetAllWhitelistedDomains :many
SELECT domain FROM content_whitelist ORDER BY domain;

-- name: AddToWhitelist :exec
INSERT OR IGNORE INTO content_whitelist (domain, created_at) VALUES (?, CURRENT_TIMESTAMP);

-- name: RemoveFromWhitelist :exec
DELETE FROM content_whitelist WHERE domain = ?;

-- name: IsWhitelisted :one
SELECT COUNT(*) FROM content_whitelist WHERE domain = ? LIMIT 1;
