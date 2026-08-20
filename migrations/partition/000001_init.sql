CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    relation TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS asset_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL CHECK (event_type IN ('INITIAL_ASSET','ASSET_IN','ASSET_OUT','ADJUSTMENT')),
    amount_cent INTEGER NOT NULL,
    holder_member_id INTEGER NOT NULL REFERENCES members(id),
    description TEXT NOT NULL DEFAULT '',
    occurred_at TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    created_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS matters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id INTEGER REFERENCES matters(id),
    title TEXT NOT NULL,
    matter_type TEXT NOT NULL DEFAULT 'general',
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','active','done','cancelled')),
    start_date TEXT,
    due_date TEXT,
    owner_member_id INTEGER REFERENCES members(id),
    created_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS public_expenses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT '',
    amount_cent INTEGER NOT NULL CHECK (amount_cent > 0),
    occurred_at TEXT NOT NULL,
    handler_member_id INTEGER NOT NULL REFERENCES members(id),
    payer_member_id INTEGER NOT NULL REFERENCES members(id),
    funding_type TEXT NOT NULL CHECK (funding_type IN ('PUBLIC_HELD_ASSET','PERSONAL_ADVANCE')),
    holder_member_id INTEGER REFERENCES members(id),
    payment_channel TEXT NOT NULL DEFAULT '',
    merchant TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    matter_id INTEGER REFERENCES matters(id),
    reimbursable_amount_cent INTEGER NOT NULL DEFAULT 0 CHECK (reimbursable_amount_cent >= 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    version INTEGER NOT NULL DEFAULT 1,
    created_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS holder_transfers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_member_id INTEGER NOT NULL REFERENCES members(id),
    to_member_id INTEGER NOT NULL REFERENCES members(id),
    amount_cent INTEGER NOT NULL CHECK (amount_cent > 0),
    purpose TEXT NOT NULL DEFAULT '',
    payment_channel TEXT NOT NULL DEFAULT '',
    occurred_at TEXT NOT NULL,
    matter_id INTEGER REFERENCES matters(id),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    created_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL,
    CHECK (from_member_id <> to_member_id)
);

CREATE TABLE IF NOT EXISTS reimbursements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    expense_id INTEGER NOT NULL REFERENCES public_expenses(id),
    payer_holder_member_id INTEGER NOT NULL REFERENCES members(id),
    receiver_member_id INTEGER NOT NULL REFERENCES members(id),
    amount_cent INTEGER NOT NULL CHECK (amount_cent > 0),
    payment_channel TEXT NOT NULL DEFAULT '',
    occurred_at TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    created_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS archives (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT '其他',
    content TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'family' CHECK (visibility IN ('family','admin')),
    created_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS attachments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    archive_id INTEGER NOT NULL REFERENCES archives(id) ON DELETE CASCADE,
    storage_name TEXT NOT NULL UNIQUE,
    original_name TEXT NOT NULL,
    mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    size INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    uploaded_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_member_id INTEGER REFERENCES members(id),
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id INTEGER,
    before_json TEXT,
    after_json TEXT,
    reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_expenses_occurred ON public_expenses(occurred_at, id);
CREATE INDEX IF NOT EXISTS idx_transfers_occurred ON holder_transfers(occurred_at, id);
CREATE INDEX IF NOT EXISTS idx_reimbursements_expense ON reimbursements(expense_id, status);
CREATE INDEX IF NOT EXISTS idx_matters_parent ON matters(parent_id);
CREATE INDEX IF NOT EXISTS idx_archives_category ON archives(category, created_at);
