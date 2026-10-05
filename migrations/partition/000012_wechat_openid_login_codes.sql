CREATE TABLE IF NOT EXISTS wechat_code_login_browser_states (
    state_hash TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL,
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_wechat_code_login_browser_state_expiry
ON wechat_code_login_browser_states(expires_at);

CREATE TABLE IF NOT EXISTS wechat_openid_login_codes (
    openid TEXT PRIMARY KEY,
    code_hash TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL DEFAULT '',
    issued_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_wechat_openid_login_active_code
ON wechat_openid_login_codes(code_hash)
WHERE code_hash <> '' AND consumed_at = '';

CREATE INDEX IF NOT EXISTS idx_wechat_openid_login_code_expiry
ON wechat_openid_login_codes(expires_at);
