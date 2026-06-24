-- name: GetRecentHistory :many
SELECT * FROM history
ORDER BY last_visited DESC, id DESC
LIMIT ? OFFSET ?;

-- name: GetRecentHistoryByDomain :many
SELECT * FROM history
WHERE domain = @domain
ORDER BY last_visited DESC, id DESC
LIMIT @limit OFFSET @offset;

-- name: GetAllRecentHistoryByDomain :many
SELECT * FROM history
WHERE domain = @domain
ORDER BY last_visited DESC, id DESC;

-- name: GetRecentHistoryWindow :many
SELECT * FROM history
WHERE last_visited < @before OR (last_visited = @before AND id < @before_id)
ORDER BY last_visited DESC, id DESC
LIMIT @limit;

-- name: GetRecentHistoryWindowByDomain :many
SELECT * FROM history
WHERE domain = @domain AND (last_visited < @before OR (last_visited = @before AND id < @before_id))
ORDER BY last_visited DESC, id DESC
LIMIT @limit;

-- name: GetHistoryByURL :one
SELECT * FROM history WHERE url = ? LIMIT 1;

-- name: UpsertHistory :exec
INSERT INTO history (url, title, favicon_url, domain)
VALUES (?, ?, ?, ?)
ON CONFLICT(url)
DO UPDATE SET
    visit_count = visit_count + 1,
    last_visited = CURRENT_TIMESTAMP,
    title = COALESCE(EXCLUDED.title, history.title),
    favicon_url = COALESCE(EXCLUDED.favicon_url, history.favicon_url),
    domain = COALESCE(EXCLUDED.domain, history.domain);

-- name: IncrementVisitCount :exec
UPDATE history
SET visit_count = visit_count + 1, last_visited = CURRENT_TIMESTAMP
WHERE url = ?;

-- name: IncrementVisitCountByDelta :exec
UPDATE history
SET visit_count = visit_count + ?, last_visited = CURRENT_TIMESTAMP
WHERE url = ?;

-- name: SearchHistory :many
SELECT * FROM history
WHERE url LIKE '%' || ? || '%' OR title LIKE '%' || ? || '%'
ORDER BY visit_count DESC, last_visited DESC
LIMIT ?;

-- name: DeleteHistoryByID :exec
DELETE FROM history WHERE id = ?;

-- name: DeleteHistoryOlderThan :exec
DELETE FROM history WHERE last_visited < ?;

-- name: DeleteHistorySince :exec
DELETE FROM history WHERE last_visited >= ?;

-- name: DeleteAllHistory :exec
DELETE FROM history;

-- name: GetHistoryStats :one
SELECT
    COUNT(*) as total_entries,
    COALESCE(SUM(visit_count), 0) as total_visits,
    COUNT(DISTINCT date(last_visited)) as unique_days
FROM history;

-- name: GetDomainStats :many
SELECT
    COALESCE(domain, '') as domain,
    COUNT(*) as page_count,
    SUM(visit_count) as total_visits,
    MAX(last_visited) as last_visit
FROM history
WHERE domain IS NOT NULL AND domain != ''
GROUP BY domain
ORDER BY total_visits DESC
LIMIT ?;

-- name: GetHourlyDistribution :many
SELECT
    CAST(strftime('%H', last_visited) AS INTEGER) as hour,
    COUNT(*) as visit_count
FROM history
GROUP BY hour
ORDER BY hour;

-- name: GetDailyVisitCount :many
SELECT
    date(last_visited) as day,
    COUNT(*) as entries,
    SUM(visit_count) as visits
FROM history
WHERE last_visited >= date('now', ?)
GROUP BY day
ORDER BY day ASC;

-- name: DeleteHistoryByDomain :exec
DELETE FROM history WHERE domain = @domain;

-- name: SearchHistoryFTSUrl :many
SELECT h.id, h.url, h.title, h.favicon_url, h.visit_count, h.last_visited, h.created_at, h.domain
FROM history_fts fts
JOIN history h ON fts.rowid = h.id
WHERE fts.url MATCH @query
ORDER BY h.visit_count DESC, h.last_visited DESC
LIMIT @limit;

-- name: SearchHistoryFTSUrlWithDomainBoost :many
SELECT h.id, h.url, h.title, h.favicon_url, h.visit_count, h.last_visited, h.created_at,
       CASE
           WHEN h.url LIKE '%://' || @term || '.%' THEN 2
           WHEN h.url LIKE '%://%.' || @term || '.%' THEN 2
           WHEN h.url LIKE '%://' || @term || '/%' THEN 2
           WHEN h.url LIKE '%://%.' || @term || '/%' THEN 1
           ELSE 0
       END as domain_boost
FROM history_fts fts
JOIN history h ON fts.rowid = h.id
WHERE fts.url MATCH @query
ORDER BY domain_boost DESC, h.visit_count DESC, h.last_visited DESC
LIMIT @limit;

-- name: SearchHistoryFTSTitle :many
SELECT h.id, h.url, h.title, h.favicon_url, h.visit_count, h.last_visited, h.created_at, h.domain
FROM history_fts fts
JOIN history h ON fts.rowid = h.id
WHERE fts.title MATCH @query
ORDER BY h.visit_count DESC, h.last_visited DESC
LIMIT @limit;

-- name: GetRecentHistorySince :many
SELECT * FROM history
WHERE last_visited >= datetime('now', ?)
ORDER BY last_visited DESC;

-- name: GetMostVisited :many
SELECT * FROM history
WHERE last_visited >= datetime('now', ?)
ORDER BY visit_count DESC, last_visited DESC;

-- name: GetAllRecentHistory :many
SELECT * FROM history
ORDER BY last_visited DESC, id DESC;

-- name: GetAllMostVisited :many
SELECT * FROM history
ORDER BY visit_count DESC, last_visited DESC;

-- name: CapVisitCount :exec
UPDATE history SET visit_count = ? WHERE url = ? AND visit_count > ?;
