-- +goose Up
-- Progreso opcional de una tarea (specs/esquema-datos/, "Progreso de
-- tareas"): entero 0–100, o NULL = sin progreso. Nullable y sin
-- default para que las filas existentes queden "sin progreso" en vez
-- de inventarles un 0. El CHECK es la última barrera; los handlers ya
-- validan el rango y responden 400 antes de llegar acá.
ALTER TABLE tasks ADD COLUMN progress INTEGER CHECK (progress IS NULL OR (progress BETWEEN 0 AND 100));

-- +goose Down
ALTER TABLE tasks DROP COLUMN progress;
