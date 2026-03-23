-- +goose Up
ALTER TABLE auth_tokens ADD COLUMN client_id TEXT;

CREATE TABLE pending_sessions (
    client_id TEXT PRIMARY KEY,
    session_value TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE pending_sessions;
