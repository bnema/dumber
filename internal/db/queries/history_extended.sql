-- Extended history queries: timeline, analytics, cleanup
-- NOTE: FTS5 search uses raw SQL in Go - sqlc doesn't support "table MATCH ?" syntax

-- name: GetHistoryTimeline :many
SELECT h.*, date(h.last_visited) as visit_date FROM history h ORDER BY h.last_visited DESC LIMIT ? OFFSET ?;

-- name: GetHistoryByDateRange :many
SELECT * FROM history WHERE last_visited >= ? AND last_visited < ? ORDER BY last_visited DESC;

-- name: GetHistoryDates :many
SELECT DISTINCT date(last_visited) as visit_date FROM history ORDER BY visit_date DESC LIMIT ?;

-- name: GetHistoryStats :one
SELECT COUNT(*) as total_entries, SUM(visit_count) as total_visits, COUNT(DISTINCT date(last_visited)) as unique_days FROM history;

-- name: GetDomainStats :many
SELECT SUBSTR(SUBSTR(url, INSTR(url, '://') + 3), 1, CASE WHEN INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') > 0 THEN INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') - 1 ELSE LENGTH(SUBSTR(url, INSTR(url, '://') + 3)) END) as domain, COUNT(*) as page_count, SUM(visit_count) as total_visits, MAX(last_visited) as last_visit FROM history GROUP BY domain ORDER BY total_visits DESC LIMIT ?;

-- name: GetHourlyDistribution :many
SELECT CAST(strftime('%H', last_visited) AS INTEGER) as hour, COUNT(*) as visit_count FROM history GROUP BY hour ORDER BY hour;

-- name: GetDailyVisitCount :many
SELECT date(last_visited) as day, COUNT(*) as entries, SUM(visit_count) as visits FROM history WHERE last_visited >= date('now', ?) GROUP BY day ORDER BY day ASC;

-- name: DeleteHistoryLastHour :exec
DELETE FROM history WHERE last_visited >= datetime('now', '-1 hour');

-- name: DeleteHistoryLastDay :exec
DELETE FROM history WHERE last_visited >= datetime('now', '-1 day');

-- name: DeleteHistoryLastWeek :exec
DELETE FROM history WHERE last_visited >= datetime('now', '-7 days');

-- name: DeleteHistoryLastMonth :exec
DELETE FROM history WHERE last_visited >= datetime('now', '-30 days');

-- name: DeleteHistoryByDomain :exec
DELETE FROM history WHERE url LIKE '%://' || ? || '/%' OR url LIKE '%://' || ? || '?%' OR url LIKE '%://' || ? || '#%' OR url LIKE '%://' || ?;

-- name: DeleteHistoryOlderThan :exec
DELETE FROM history WHERE last_visited < datetime('now', '-' || CAST(? AS TEXT) || ' days');

-- name: SetFavoriteShortcut :exec
UPDATE favorites SET shortcut_key = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: ClearFavoriteShortcut :exec
UPDATE favorites SET shortcut_key = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: GetFavoriteByShortcut :one
SELECT * FROM favorites WHERE shortcut_key = ? LIMIT 1;

-- name: ClearShortcutFromOthers :exec
UPDATE favorites SET shortcut_key = NULL WHERE shortcut_key = ?;

-- name: SetFavoriteFolder :exec
UPDATE favorites SET folder_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: ClearFavoriteFolder :exec
UPDATE favorites SET folder_id = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?;
