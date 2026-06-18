-- +goose Up
ALTER TABLE sessions ADD COLUMN process_id INTEGER;

-- +goose Down
-- SQLite cannot drop columns portably in older versions; keep the nullable metadata column.
