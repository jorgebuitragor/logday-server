-- +goose Up
-- Registro de aceptación de la política por usuario
-- (specs/cumplimiento-datos-personales/). NULL = nunca aceptó nada
-- todavía — toda cuenta existente antes de esta migración queda así,
-- y ve el gate de consentimiento en su próximo login, igual que
-- cualquier usuario nuevo.
ALTER TABLE users ADD COLUMN privacy_accepted_version INTEGER;
ALTER TABLE users ADD COLUMN privacy_accepted_at TEXT;
-- Un solo flag alcanza: hoy el único dato sensible es "incapacidad" en
-- absence_days, no hace falta versionar este consentimiento aparte.
ALTER TABLE users ADD COLUMN sensitive_data_accepted_at TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN privacy_accepted_version;
ALTER TABLE users DROP COLUMN privacy_accepted_at;
ALTER TABLE users DROP COLUMN sensitive_data_accepted_at;
