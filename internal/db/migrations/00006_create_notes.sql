-- +goose Up
-- content is a plain TEXT column for now (LWW by whole row, same
-- simplification as tasks.content) — it becomes content_crdt BLOB
-- once the yrs/CGO CRDT integration is built (see
-- specs/arquitectura-inicial and specs/esquema-datos).
CREATE TABLE notes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    title TEXT NOT NULL,
    folder TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '[]',
    created TEXT NOT NULL,
    updated TEXT NOT NULL,
    pinned BOOLEAN NOT NULL DEFAULT 0,
    content TEXT NOT NULL DEFAULT '',
    seq INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX idx_notes_user_id_seq ON notes (user_id, seq);

-- +goose Down
DROP TABLE notes;
