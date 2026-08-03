-- +goose Up
-- Backs the per-user monotonic seq assigned to every write across all
-- domain tables (see specs/sync-incremental).
CREATE TABLE user_sync_counters (
    user_id TEXT PRIMARY KEY,
    next_seq INTEGER NOT NULL DEFAULT 1
);

-- +goose Down
DROP TABLE user_sync_counters;
