-- +goose Up
-- Keyed by (user_id, year_month) — no client-generated id, unlike
-- most domain tables (see specs/esquema-datos, "OvertimeMonthMeta").
-- year_month acts as the synthetic id in REST URLs and sync changes.
CREATE TABLE overtime_month_meta (
    user_id TEXT NOT NULL,
    year_month TEXT NOT NULL,
    colaborador TEXT NOT NULL DEFAULT '',
    cedula TEXT NOT NULL DEFAULT '',
    seq INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT,
    PRIMARY KEY (user_id, year_month)
);

CREATE INDEX idx_overtime_month_meta_user_id_seq ON overtime_month_meta (user_id, seq);

-- +goose Down
DROP TABLE overtime_month_meta;
