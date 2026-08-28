ALTER TABLE passkey_login_credentials ADD COLUMN member_id INTEGER REFERENCES members(id);
ALTER TABLE passkey_login_sessions ADD COLUMN member_id INTEGER REFERENCES members(id);

CREATE INDEX IF NOT EXISTS idx_passkey_login_credentials_member
ON passkey_login_credentials(member_id, identity_id, id);

ALTER TABLE medication_plans ADD COLUMN timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai';
ALTER TABLE medication_plans ADD COLUMN ended_at TEXT;
ALTER TABLE medication_plans ADD COLUMN ended_by INTEGER REFERENCES members(id);

CREATE INDEX IF NOT EXISTS idx_medication_plans_reminder_v3
ON medication_plans(is_deleted, ended_at, start_date, end_date, scheduled_time, id);

CREATE TABLE IF NOT EXISTS member_notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recipient_member_id INTEGER NOT NULL REFERENCES members(id),
    actor_member_id INTEGER REFERENCES members(id),
    kind TEXT NOT NULL CHECK(kind IN ('medication_manual','medication_later')),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    link TEXT NOT NULL DEFAULT '',
    plan_id INTEGER REFERENCES medication_plans(id),
    scheduled_date TEXT NOT NULL DEFAULT '',
    read_at TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_member_notifications_recipient
ON member_notifications(recipient_member_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_member_notifications_unread
ON member_notifications(recipient_member_id, read_at, created_at DESC, id DESC);

-- 000009 的 fallback 超级审计没有对应 audit_logs 事实源，属于按 HTTP 请求猜测的“增删改”。
-- 本轮改为只投影真实 audit_logs，因此清理旧 fallback 误报。
DELETE FROM super_audit_logs WHERE audit_log_id IS NULL;

-- 访问记录只保留页面/API；清理已经写入的前端静态资源噪声。
DELETE FROM member_access_logs
WHERE path LIKE '/static/%'
   OR path = '/healthz'
   OR path = '/favicon.ico'
   OR path = '/medication-sw.js'
   OR lower(path) GLOB '*.css*'
   OR lower(path) GLOB '*.js*'
   OR lower(path) GLOB '*.map*'
   OR lower(path) GLOB '*.png*'
   OR lower(path) GLOB '*.jpg*'
   OR lower(path) GLOB '*.jpeg*'
   OR lower(path) GLOB '*.gif*'
   OR lower(path) GLOB '*.webp*'
   OR lower(path) GLOB '*.svg*'
   OR lower(path) GLOB '*.ico*'
   OR lower(path) GLOB '*.woff*'
   OR lower(path) GLOB '*.woff2*'
   OR lower(path) GLOB '*.ttf*';

-- 自动提醒失败不能永久占住阶段。只有成功投递才应阻止该阶段再次尝试。
DROP INDEX IF EXISTS idx_medication_delivery_auto_once;
CREATE UNIQUE INDEX IF NOT EXISTS idx_medication_delivery_auto_sent_once
ON medication_notification_deliveries(plan_id, scheduled_date, stage, channel)
WHERE stage <> 'manual' AND status = 'sent';
