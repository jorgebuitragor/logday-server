-- +goose Up
CREATE TABLE absence_days (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    date TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('incapacidad', 'vacaciones', 'otro')),
    note TEXT,
    seq INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX idx_absence_days_user_id_seq ON absence_days (user_id, seq);

-- +goose Down
DROP TABLE absence_days;
