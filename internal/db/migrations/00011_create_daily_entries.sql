-- +goose Up
-- Keyed by (user_id, date) — no client-generated id, same shape as
-- overtime_month_meta. content is plain TEXT for now (LWW by row),
-- not content_crdt — same deliberate deviation as notes.content, see
-- specs/arquitectura-inicial ("Resolución de conflictos").
CREATE TABLE daily_entries (
    user_id TEXT NOT NULL,
    date TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    seq INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    PRIMARY KEY (user_id, date)
);

CREATE INDEX idx_daily_entries_user_id_seq ON daily_entries (user_id, seq);

-- +goose Down
DROP TABLE daily_entries;
