-- +goose Up
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('browser', 'cli')),
    started_at TIMESTAMP NOT NULL,
    ended_at TIMESTAMP
);

CREATE INDEX idx_sessions_started_at ON sessions(started_at DESC);
CREATE INDEX idx_sessions_active ON sessions(started_at DESC) WHERE ended_at IS NULL;

-- +goose Down
DROP TABLE sessions;
