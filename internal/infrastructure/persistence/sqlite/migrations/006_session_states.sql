-- +goose Up
CREATE TABLE session_states (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    state_json TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    tab_count INTEGER NOT NULL DEFAULT 0,
    pane_count INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_session_states_updated_at ON session_states(updated_at DESC);

-- +goose Down
DROP TABLE session_states;
