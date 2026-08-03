-- +goose Up
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    title TEXT NOT NULL,
    task_code TEXT,
    status TEXT NOT NULL CHECK (status IN ('todo', 'in-progress', 'done')),
    tags TEXT NOT NULL DEFAULT '[]',
    project TEXT NOT NULL DEFAULT '',
    created TEXT NOT NULL,
    completed_at TEXT,
    due TEXT,
    content TEXT NOT NULL DEFAULT '',
    seq INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX idx_tasks_user_id_seq ON tasks (user_id, seq);

-- +goose Down
DROP TABLE tasks;
