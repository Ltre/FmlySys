CREATE TABLE IF NOT EXISTS passkey_login_identities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    phone TEXT NOT NULL UNIQUE,
    profile_remark TEXT NOT NULL,
    member_id INTEGER REFERENCES members(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_passkey_login_identities_member
ON passkey_login_identities(member_id);

CREATE TABLE IF NOT EXISTS passkey_login_users (
    identity_id INTEGER NOT NULL REFERENCES passkey_login_identities(id) ON DELETE CASCADE,
    rp_id TEXT NOT NULL,
    user_handle BLOB NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY(identity_id, rp_id),
    UNIQUE(rp_id, user_handle)
);

CREATE TABLE IF NOT EXISTS passkey_login_credentials (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    identity_id INTEGER NOT NULL REFERENCES passkey_login_identities(id) ON DELETE CASCADE,
    rp_id TEXT NOT NULL,
    credential_id BLOB NOT NULL,
    credential_json TEXT NOT NULL,
    flags INTEGER NOT NULL DEFAULT 0,
    remark TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_used_at TEXT NOT NULL DEFAULT '',
    UNIQUE(rp_id, credential_id),
    FOREIGN KEY(identity_id, rp_id) REFERENCES passkey_login_users(identity_id, rp_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_passkey_login_credentials_identity
ON passkey_login_credentials(identity_id, rp_id);

CREATE TABLE IF NOT EXISTS passkey_login_ceremonies (
    token_hash TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK(kind IN ('create','login','add')),
    identity_id INTEGER REFERENCES passkey_login_identities(id) ON DELETE CASCADE,
    rp_id TEXT NOT NULL,
    phone TEXT NOT NULL DEFAULT '',
    remark TEXT NOT NULL DEFAULT '',
    session_json TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_passkey_login_ceremonies_expiry
ON passkey_login_ceremonies(expires_at);

CREATE TABLE IF NOT EXISTS passkey_login_sessions (
    token_hash TEXT PRIMARY KEY,
    identity_id INTEGER NOT NULL REFERENCES passkey_login_identities(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    verified_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_passkey_login_sessions_identity
ON passkey_login_sessions(identity_id, expires_at);
