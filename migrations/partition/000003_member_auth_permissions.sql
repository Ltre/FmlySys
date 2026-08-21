CREATE TABLE IF NOT EXISTS wechat_identities (
    openid TEXT PRIMARY KEY,
    unionid TEXT NOT NULL DEFAULT '',
    nickname TEXT NOT NULL DEFAULT '',
    member_id INTEGER REFERENCES members(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_wechat_identities_member
ON wechat_identities(member_id);

CREATE TABLE IF NOT EXISTS join_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    openid TEXT NOT NULL UNIQUE,
    unionid TEXT NOT NULL DEFAULT '',
    nickname TEXT NOT NULL DEFAULT '',
    real_name TEXT NOT NULL DEFAULT '',
    relation TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','pending','approved','rejected')),
    rejection_reason TEXT NOT NULL DEFAULT '',
    access_token_hash TEXT NOT NULL DEFAULT '',
    access_token_expires_at TEXT NOT NULL DEFAULT '',
    requested_at TEXT NOT NULL DEFAULT '',
    reviewed_at TEXT NOT NULL DEFAULT '',
    reviewed_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_join_requests_status
ON join_requests(status, requested_at, id);

CREATE TABLE IF NOT EXISTS member_permissions (
    member_id INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY(member_id, permission)
);

CREATE TABLE IF NOT EXISTS member_sessions (
    token_hash TEXT PRIMARY KEY,
    member_id INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_member_sessions_member
ON member_sessions(member_id, expires_at);
