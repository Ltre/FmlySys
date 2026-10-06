CREATE TABLE IF NOT EXISTS totp_login_identity_sessions (
    token_hash TEXT PRIMARY KEY,
    identity_id INTEGER NOT NULL REFERENCES totp_login_identities(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    verified_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_totp_login_identity_sessions_identity
ON totp_login_identity_sessions(identity_id, expires_at);
