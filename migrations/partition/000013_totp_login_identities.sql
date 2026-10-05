CREATE TABLE IF NOT EXISTS totp_login_identities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    username_key TEXT NOT NULL UNIQUE,
    profile_remark TEXT NOT NULL DEFAULT '',
    secret_enc TEXT NOT NULL,
    last_totp_step INTEGER NOT NULL DEFAULT -1,
    member_id INTEGER REFERENCES members(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_totp_login_identities_member
ON totp_login_identities(member_id);

CREATE TABLE IF NOT EXISTS totp_login_enrollments (
    token_hash TEXT PRIMARY KEY,
    secret_enc TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_totp_login_enrollments_expiry
ON totp_login_enrollments(expires_at);
