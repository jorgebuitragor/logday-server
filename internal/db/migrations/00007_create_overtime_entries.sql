-- +goose Up
CREATE TABLE overtime_entries (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    fecha TEXT NOT NULL,
    solicitada_por TEXT NOT NULL DEFAULT '',
    actividad TEXT NOT NULL DEFAULT '',
    observaciones TEXT NOT NULL DEFAULT '',
    hora_inicio TEXT NOT NULL DEFAULT '',
    hora_final TEXT NOT NULL DEFAULT '',
    total_horas REAL NOT NULL DEFAULT 0,
    extras_diurnas REAL NOT NULL DEFAULT 0,
    extras_nocturnas REAL NOT NULL DEFAULT 0,
    extras_diurnas_festivas REAL NOT NULL DEFAULT 0,
    extras_nocturnas_festivas REAL NOT NULL DEFAULT 0,
    seq INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX idx_overtime_entries_user_id_seq ON overtime_entries (user_id, seq);

-- +goose Down
DROP TABLE overtime_entries;
