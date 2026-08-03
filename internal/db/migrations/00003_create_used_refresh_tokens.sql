-- +goose Up
-- Records refresh tokens that were already rotated away, so a later
-- attempt to reuse one can be detected as possible token theft instead
-- of silently failing like any other invalid token.
CREATE TABLE used_refresh_tokens (
    token_hash TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    used_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE used_refresh_tokens;
