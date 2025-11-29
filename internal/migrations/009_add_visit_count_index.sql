-- Add composite index for GetMostVisited query performance
-- Orders by visit_count DESC, last_visited DESC

CREATE INDEX IF NOT EXISTS idx_history_visit_count_last_visited
ON history(visit_count DESC, last_visited DESC);
