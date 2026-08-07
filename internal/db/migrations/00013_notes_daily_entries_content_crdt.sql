-- +goose Up
-- Replaces the interim plain-TEXT content column with real CRDT
-- storage (Deln0r/ygo, a pure-Go Yjs-wire-compatible port — see
-- specs/arquitectura-inicial, "Resolución de conflictos"). Breaking
-- schema change, acceptable pre-launch: no live client integration
-- exists yet to migrate data for.
ALTER TABLE notes DROP COLUMN content;
ALTER TABLE notes ADD COLUMN content_crdt BLOB;

ALTER TABLE daily_entries DROP COLUMN content;
ALTER TABLE daily_entries ADD COLUMN content_crdt BLOB;

-- +goose Down
ALTER TABLE notes DROP COLUMN content_crdt;
ALTER TABLE notes ADD COLUMN content TEXT NOT NULL DEFAULT '';

ALTER TABLE daily_entries DROP COLUMN content_crdt;
ALTER TABLE daily_entries ADD COLUMN content TEXT NOT NULL DEFAULT '';
