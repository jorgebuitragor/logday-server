-- +goose Up
-- IP y User-Agent del último uso de cada device, usado por el panel de
-- administración (specs/panel-admin/) para mostrar de dónde se conecta
-- cada sesión y para inferir un ícono de tipo de dispositivo a partir del
-- User-Agent. Se actualizan en login (creación) y en cada refresh
-- (rotateRefreshToken) junto con last_used_at, así que reflejan la
-- conexión más reciente, no la original.
ALTER TABLE devices ADD COLUMN last_ip TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE devices DROP COLUMN last_ip;
ALTER TABLE devices DROP COLUMN user_agent;
