-- +goose Up
-- Seis columnas más de instance_settings (specs/panel-admin/), todas con
-- default idéntico al valor que hoy está hardcodeado en el código Go —
-- una instancia existente se comporta exactamente igual hasta que un
-- admin cambie algo desde el panel.
ALTER TABLE instance_settings ADD COLUMN allowed_email_domains TEXT NOT NULL DEFAULT '';
ALTER TABLE instance_settings ADD COLUMN min_password_length INTEGER NOT NULL DEFAULT 8;
ALTER TABLE instance_settings ADD COLUMN access_token_ttl_minutes INTEGER NOT NULL DEFAULT 15;
ALTER TABLE instance_settings ADD COLUMN refresh_token_ttl_days INTEGER NOT NULL DEFAULT 30;
ALTER TABLE instance_settings ADD COLUMN panel_session_ttl_hours INTEGER NOT NULL DEFAULT 24;
ALTER TABLE instance_settings ADD COLUMN max_devices_per_user INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE instance_settings DROP COLUMN allowed_email_domains;
ALTER TABLE instance_settings DROP COLUMN min_password_length;
ALTER TABLE instance_settings DROP COLUMN access_token_ttl_minutes;
ALTER TABLE instance_settings DROP COLUMN refresh_token_ttl_days;
ALTER TABLE instance_settings DROP COLUMN panel_session_ttl_hours;
ALTER TABLE instance_settings DROP COLUMN max_devices_per_user;
