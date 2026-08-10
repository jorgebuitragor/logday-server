-- +goose Up
-- Singleton row (id is CHECK'd to always be 1) holding instance-wide
-- operational config that used to be hardcoded constants — surfaced in
-- the admin panel's "Configuración" section (specs/panel-admin/). Not a
-- per-user or synced table, so it deliberately doesn't follow the
-- (user_id, seq, deleted_at) shape the 7 domain tables use.
CREATE TABLE instance_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    instance_name TEXT NOT NULL DEFAULT 'Logday Server',
    tombstone_retention_days INTEGER NOT NULL DEFAULT 90,
    login_rate_limit_attempts INTEGER NOT NULL DEFAULT 5,
    login_rate_limit_window_seconds INTEGER NOT NULL DEFAULT 60,
    updated_at TEXT NOT NULL
);

INSERT INTO instance_settings
    (id, instance_name, tombstone_retention_days, login_rate_limit_attempts, login_rate_limit_window_seconds, updated_at)
VALUES
    (1, 'Logday Server', 90, 5, 60, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

-- +goose Down
DROP TABLE instance_settings;
