ALTER TABLE medication_plans ADD COLUMN is_deleted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE medication_plans ADD COLUMN deleted_at TEXT;
ALTER TABLE medication_plans ADD COLUMN deleted_by INTEGER REFERENCES members(id);

CREATE INDEX IF NOT EXISTS idx_medication_plans_visible
ON medication_plans(is_deleted, patient_member_id, start_date, end_date, scheduled_time, id);

INSERT OR IGNORE INTO member_permissions(member_id, permission, created_at)
SELECT member_id, 'medication.manage_self', created_at
FROM member_permissions
WHERE permission = 'medication.manage';

INSERT OR IGNORE INTO member_permissions(member_id, permission, created_at)
SELECT member_id, 'medication.manage_others', created_at
FROM member_permissions
WHERE permission = 'medication.manage';

INSERT OR IGNORE INTO member_permissions(member_id, permission, created_at)
SELECT member_id, 'medication.view', created_at
FROM member_permissions
WHERE permission = 'medication.manage';

DELETE FROM member_permissions WHERE permission = 'medication.manage';

CREATE TABLE IF NOT EXISTS member_access_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ip_address TEXT NOT NULL,
    member_id INTEGER NOT NULL REFERENCES members(id),
    member_name TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    accessed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_member_access_logs_time
ON member_access_logs(accessed_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_member_access_logs_member
ON member_access_logs(member_id, accessed_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS super_audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    audit_log_id INTEGER UNIQUE,
    surface TEXT NOT NULL CHECK (surface IN ('front','admin')),
    ip_address TEXT NOT NULL,
    member_id INTEGER REFERENCES members(id),
    member_name TEXT NOT NULL DEFAULT '',
    admin_username TEXT NOT NULL DEFAULT '',
    operation TEXT NOT NULL CHECK (operation IN ('create','update','delete')),
    data_category TEXT NOT NULL,
    original_action TEXT NOT NULL,
    entity_id INTEGER,
    before_json TEXT,
    after_json TEXT,
    request_method TEXT NOT NULL DEFAULT '',
    request_path TEXT NOT NULL DEFAULT '',
    operation_time TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_super_audit_surface_time
ON super_audit_logs(surface, operation_time DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_super_audit_member_time
ON super_audit_logs(member_id, operation_time DESC, id DESC);

CREATE TABLE IF NOT EXISTS medication_checkins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL REFERENCES medication_plans(id),
    scheduled_date TEXT NOT NULL,
    patient_member_id INTEGER NOT NULL REFERENCES members(id),
    response TEXT NOT NULL CHECK (response IN ('taken','later')),
    response_at TEXT NOT NULL,
    verification_status TEXT NOT NULL DEFAULT 'none' CHECK (verification_status IN ('none','pending','confirmed','rejected')),
    verified_by_member_id INTEGER REFERENCES members(id),
    verified_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE(plan_id, scheduled_date)
);

CREATE INDEX IF NOT EXISTS idx_medication_checkins_pending
ON medication_checkins(verification_status, scheduled_date, plan_id);

CREATE TABLE IF NOT EXISTS medication_push_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER NOT NULL REFERENCES members(id),
    endpoint TEXT NOT NULL UNIQUE,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_medication_push_member
ON medication_push_subscriptions(member_id, id);

CREATE TABLE IF NOT EXISTS medication_notification_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL REFERENCES medication_plans(id),
    scheduled_date TEXT NOT NULL,
    stage TEXT NOT NULL CHECK (stage IN ('scheduled','plus1h','plus2h','manual')),
    channel TEXT NOT NULL CHECK (channel IN ('pwa','termux')),
    status TEXT NOT NULL CHECK (status IN ('sent','failed')),
    detail TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_medication_delivery_plan_date
ON medication_notification_deliveries(plan_id, scheduled_date, stage, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_medication_delivery_auto_once
ON medication_notification_deliveries(plan_id, scheduled_date, stage, channel)
WHERE stage <> 'manual';
