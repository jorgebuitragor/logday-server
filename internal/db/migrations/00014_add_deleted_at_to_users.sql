-- +goose Up
-- Soft-delete para users, usado por el panel de administración
-- (specs/panel-admin/) para dar de baja/restaurar cuentas sin perder el
-- historial. A diferencia de las 7 tablas de dominio sincronizadas, este
-- deleted_at no se propaga como tombstone a ningún cliente — solo controla
-- si la cuenta puede autenticarse.
ALTER TABLE users ADD COLUMN deleted_at TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN deleted_at;
