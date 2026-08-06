-- +goose Up
-- Tracks, per user, the highest seq among tombstones ever physically
-- purged (see internal/db/purge.go). A cursor requesting changes
-- since a seq below this value can no longer be answered completely
-- — some now-deleted tombstones with a higher seq than that cursor
-- were removed — so GET /sync/changes must reject it (410) instead of
-- silently omitting them.
ALTER TABLE user_sync_counters ADD COLUMN purged_before_seq INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_sync_counters DROP COLUMN purged_before_seq;
