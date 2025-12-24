-- name: UpsertSessionState :exec
INSERT INTO session_states (session_id, state_json, version, tab_count, pane_count, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    state_json = excluded.state_json,
    version = excluded.version,
    tab_count = excluded.tab_count,
    pane_count = excluded.pane_count,
    updated_at = excluded.updated_at;

-- name: GetSessionState :one
SELECT session_id, state_json, version, tab_count, pane_count, updated_at
FROM session_states
WHERE session_id = ?;

-- name: GetAllSessionStates :many
SELECT session_id, state_json, version, tab_count, pane_count, updated_at
FROM session_states
ORDER BY updated_at DESC;

-- name: DeleteSessionState :exec
DELETE FROM session_states WHERE session_id = ?;

-- name: GetSessionsWithState :many
SELECT 
    s.id, s.type, s.started_at, s.ended_at,
    ss.state_json, ss.tab_count, ss.pane_count, ss.updated_at as state_updated_at
FROM sessions s
LEFT JOIN session_states ss ON s.id = ss.session_id
WHERE s.type = 'browser'
ORDER BY s.started_at DESC
LIMIT ?;

-- name: GetTotalSessionStatesSize :one
SELECT COALESCE(SUM(LENGTH(state_json)), 0) as total_size
FROM session_states;
