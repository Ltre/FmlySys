CREATE TABLE IF NOT EXISTS passkey_users (
    member_id INTEGER NOT NULL,
    rp_id TEXT NOT NULL,
    user_handle BLOB NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY(member_id, rp_id),
    UNIQUE(rp_id, user_handle),
    FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS passkey_credentials (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER NOT NULL,
    rp_id TEXT NOT NULL,
    credential_id BLOB NOT NULL,
    credential_json TEXT NOT NULL,
    flags INTEGER NOT NULL DEFAULT 0,
    remark TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_used_at TEXT NOT NULL DEFAULT '',
    UNIQUE(rp_id, credential_id),
    FOREIGN KEY(member_id, rp_id) REFERENCES passkey_users(member_id, rp_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_passkey_credentials_member ON passkey_credentials(member_id, rp_id);

CREATE TABLE IF NOT EXISTS passkey_ceremonies (
    token_hash TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK(kind IN ('register','login')),
    member_id INTEGER,
    rp_id TEXT NOT NULL,
    session_json TEXT NOT NULL,
    remark TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_passkey_ceremonies_expiry ON passkey_ceremonies(expires_at);
