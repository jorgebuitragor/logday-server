-- +goose Up
-- Per-field LWW timestamps (see specs/lww-por-campo) — a JSON map of
-- field name -> last write's updated_at, so PATCH can resolve
-- conflicts per field instead of per row. Existing rows get an empty
-- map: the first PATCH to any of their fields after this migration
-- wins unconditionally, since there's no real per-field history to
-- compare against a single row-level updated_at.
ALTER TABLE tasks ADD COLUMN field_updated_at TEXT NOT NULL DEFAULT '{}';
ALTER TABLE notes ADD COLUMN field_updated_at TEXT NOT NULL DEFAULT '{}';
ALTER TABLE overtime_entries ADD COLUMN field_updated_at TEXT NOT NULL DEFAULT '{}';
ALTER TABLE overtime_month_meta ADD COLUMN field_updated_at TEXT NOT NULL DEFAULT '{}';
ALTER TABLE calendar_events ADD COLUMN field_updated_at TEXT NOT NULL DEFAULT '{}';
ALTER TABLE absence_days ADD COLUMN field_updated_at TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE tasks DROP COLUMN field_updated_at;
ALTER TABLE notes DROP COLUMN field_updated_at;
ALTER TABLE overtime_entries DROP COLUMN field_updated_at;
ALTER TABLE overtime_month_meta DROP COLUMN field_updated_at;
ALTER TABLE calendar_events DROP COLUMN field_updated_at;
ALTER TABLE absence_days DROP COLUMN field_updated_at;
