-- name: GetAllWhitelistedDomains :many
-- Get all whitelisted domains for content filtering
SELECT domain FROM content_whitelist ORDER BY domain;

-- name: AddToWhitelist :exec
-- Add a domain to the content filtering whitelist
INSERT OR IGNORE INTO content_whitelist (domain, created_at)
VALUES (?, CURRENT_TIMESTAMP);

-- name: RemoveFromWhitelist :exec
-- Remove a domain from the content filtering whitelist
DELETE FROM content_whitelist WHERE domain = ?;

-- name: IsWhitelisted :one
-- Check if a domain is whitelisted
SELECT COUNT(*) as count FROM content_whitelist WHERE domain = ? LIMIT 1;
