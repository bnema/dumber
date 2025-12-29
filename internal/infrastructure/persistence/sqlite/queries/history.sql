-- name: GetRecentHistory :many
SELECT * FROM history ORDER BY last_visited DESC LIMIT ? OFFSET ?;

-- name: GetHistoryByURL :one
SELECT * FROM history WHERE url = ? LIMIT 1;

-- name: UpsertHistory :exec
INSERT INTO history (url, title, favicon_url)
VALUES (?, ?, ?)
ON CONFLICT(url)
DO UPDATE SET
    visit_count = visit_count + 1,
    last_visited = CURRENT_TIMESTAMP,
    title = COALESCE(EXCLUDED.title, history.title),
    favicon_url = COALESCE(EXCLUDED.favicon_url, history.favicon_url);

-- name: IncrementVisitCount :exec
UPDATE history
SET visit_count = visit_count + 1, last_visited = CURRENT_TIMESTAMP
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

-- name: DeleteAllHistory :exec
DELETE FROM history;

-- name: GetHistoryStats :one
SELECT COUNT(*) as total_entries, COALESCE(SUM(visit_count), 0) as total_visits, COUNT(DISTINCT date(last_visited)) as unique_days FROM history;

-- name: GetDomainStats :many
SELECT SUBSTR(SUBSTR(url, INSTR(url, '://') + 3), 1, CASE WHEN INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') > 0 THEN INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') - 1 ELSE LENGTH(SUBSTR(url, INSTR(url, '://') + 3)) END) as domain, COUNT(*) as page_count, SUM(visit_count) as total_visits, MAX(last_visited) as last_visit FROM history GROUP BY domain ORDER BY total_visits DESC LIMIT ?;

-- name: GetHourlyDistribution :many
SELECT CAST(strftime('%H', last_visited) AS INTEGER) as hour, COUNT(*) as visit_count FROM history GROUP BY hour ORDER BY hour;

-- name: GetDailyVisitCount :many
SELECT date(last_visited) as day, COUNT(*) as entries, SUM(visit_count) as visits FROM history WHERE last_visited >= date('now', ?) GROUP BY day ORDER BY day ASC;

-- name: DeleteHistoryByDomain :exec
DELETE FROM history WHERE url LIKE '%://' || ? || '/%' OR url LIKE '%://' || ? || '?%' OR url LIKE '%://' || ? || '#%' OR url LIKE '%://' || ?;

-- name: SearchHistoryFTS :many
SELECT h.id, h.url, h.title, h.favicon_url, h.visit_count, h.last_visited, h.created_at
FROM history_fts fts
JOIN history h ON fts.rowid = h.id
WHERE fts.url MATCH ?
ORDER BY bm25(history_fts)
LIMIT ?;

-- name: GetRecentHistorySince :many
SELECT * FROM history
WHERE last_visited >= datetime('now', ?)
ORDER BY last_visited DESC;

-- name: GetMostVisited :many
SELECT * FROM history
WHERE last_visited >= datetime('now', ?)
ORDER BY visit_count DESC, last_visited DESC;
