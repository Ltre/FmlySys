CREATE TABLE IF NOT EXISTS admin_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret_enc TEXT NOT NULL DEFAULT '',
    totp_confirmed INTEGER NOT NULL DEFAULT 0 CHECK (totp_confirmed IN (0,1)),
    last_totp_step INTEGER NOT NULL DEFAULT -1,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS admin_sessions (
    token_hash TEXT PRIMARY KEY,
    admin_user_id INTEGER NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    stage TEXT NOT NULL CHECK (stage IN ('totp_setup','totp_verify','authenticated')),
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_admin_sessions_user
ON admin_sessions(admin_user_id, expires_at);
