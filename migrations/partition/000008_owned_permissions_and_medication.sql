INSERT OR IGNORE INTO member_permissions(member_id, permission, created_at)
SELECT member_id, 'matters.manage_self', created_at
FROM member_permissions
WHERE permission = 'matters.manage';

INSERT OR IGNORE INTO member_permissions(member_id, permission, created_at)
SELECT member_id, 'matters.manage_others', created_at
FROM member_permissions
WHERE permission = 'matters.manage';

INSERT OR IGNORE INTO member_permissions(member_id, permission, created_at)
SELECT member_id, 'matters.view', created_at
FROM member_permissions
WHERE permission = 'matters.manage';

INSERT OR IGNORE INTO member_permissions(member_id, permission, created_at)
SELECT member_id, 'share.manage_self', created_at
FROM member_permissions
WHERE permission = 'share.manage';

INSERT OR IGNORE INTO member_permissions(member_id, permission, created_at)
SELECT member_id, 'share.manage_others', created_at
FROM member_permissions
WHERE permission = 'share.manage';

INSERT OR IGNORE INTO member_permissions(member_id, permission, created_at)
SELECT member_id, 'share.view', created_at
FROM member_permissions
WHERE permission = 'share.manage';

DELETE FROM member_permissions
WHERE permission IN ('matters.manage', 'share.manage');

CREATE TABLE IF NOT EXISTS medication_plans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    patient_member_id INTEGER NOT NULL REFERENCES members(id),
    medicine_name TEXT NOT NULL,
    dosage TEXT NOT NULL,
    scheduled_time TEXT NOT NULL,
    instructions TEXT NOT NULL DEFAULT '',
    start_date TEXT NOT NULL,
    end_date TEXT,
    created_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_medication_plans_patient_schedule
ON medication_plans(patient_member_id, start_date, end_date, scheduled_time, id);

CREATE TABLE IF NOT EXISTS medication_intake_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL REFERENCES medication_plans(id),
    scheduled_date TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('taken', 'missed')),
    note TEXT NOT NULL DEFAULT '',
    recorded_by_member_id INTEGER NOT NULL REFERENCES members(id),
    recorded_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(plan_id, scheduled_date)
);

CREATE INDEX IF NOT EXISTS idx_medication_records_date
ON medication_intake_records(scheduled_date, status, plan_id);
