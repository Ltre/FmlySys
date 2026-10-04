CREATE TABLE IF NOT EXISTS wechat_code_login_attempts (
    state_hash TEXT PRIMARY KEY,
    scene_hash TEXT NOT NULL UNIQUE,
    openid TEXT NOT NULL DEFAULT '',
    code_hash TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    code_expires_at TEXT NOT NULL DEFAULT '',
    code_issued_at TEXT NOT NULL DEFAULT '',
    scanned_at TEXT NOT NULL DEFAULT '',
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    consumed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_wechat_code_login_openid
ON wechat_code_login_attempts(openid, scanned_at, expires_at);
