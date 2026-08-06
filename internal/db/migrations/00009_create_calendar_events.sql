-- +goose Up
CREATE TABLE calendar_events (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    title TEXT NOT NULL,
    date TEXT NOT NULL,
    time TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL CHECK (color IN ('indigo', 'amber', 'emerald', 'rose', 'sky', 'violet')),
    reminder_minutes INTEGER NOT NULL DEFAULT 0,
    repeat TEXT NOT NULL CHECK (repeat IN ('none', 'daily', 'weekly', 'biweekly', 'monthly', 'yearly')),
    seq INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX idx_calendar_events_user_id_seq ON calendar_events (user_id, seq);

-- +goose Down
DROP TABLE calendar_events;
