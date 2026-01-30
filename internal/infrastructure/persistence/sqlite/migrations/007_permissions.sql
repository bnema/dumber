-- +goose Up
-- Permission storage for media access (mic, camera, etc.)
-- Per W3C spec: display capture permissions cannot be persisted

CREATE TABLE IF NOT EXISTS permissions (
    origin TEXT NOT NULL,
    permission_type TEXT NOT NULL,
    decision TEXT NOT NULL CHECK(decision IN ('granted', 'denied')),
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (origin, permission_type)
);

CREATE INDEX IF NOT EXISTS idx_permissions_origin ON permissions(origin);
CREATE INDEX IF NOT EXISTS idx_permissions_updated_at ON permissions(updated_at);

-- +goose Down
DROP TABLE IF EXISTS permissions;
