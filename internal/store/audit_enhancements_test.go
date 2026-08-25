package store

import (
	"context"
	"testing"
)

func TestAccessAndSuperAuditProjection(t *testing.T) {
	s := newMedicationTestStore(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(`
CREATE TABLE member_access_logs(id INTEGER PRIMARY KEY AUTOINCREMENT,ip_address TEXT NOT NULL,member_id INTEGER NOT NULL REFERENCES members(id),member_name TEXT NOT NULL,method TEXT NOT NULL,path TEXT NOT NULL,accessed_at TEXT NOT NULL);
CREATE TABLE super_audit_logs(id INTEGER PRIMARY KEY AUTOINCREMENT,audit_log_id INTEGER UNIQUE,surface TEXT NOT NULL,ip_address TEXT NOT NULL,member_id INTEGER REFERENCES members(id),member_name TEXT NOT NULL DEFAULT '',admin_username TEXT NOT NULL DEFAULT '',operation TEXT NOT NULL,data_category TEXT NOT NULL,original_action TEXT NOT NULL,entity_id INTEGER,before_json TEXT,after_json TEXT,request_method TEXT NOT NULL DEFAULT '',request_path TEXT NOT NULL DEFAULT '',operation_time TEXT NOT NULL);`); err != nil { t.Fatal(err) }
	m := Member{ID: 2, Name: "家属甲"}
	if err := s.RecordMemberAccess(ctx, "203.0.113.10", m, "GET", "/matters"); err != nil { t.Fatal(err) }
	before, err := s.MaxAuditLogID(ctx); if err != nil { t.Fatal(err) }
	if err := s.AuditChange(ctx, 2, "update", "matter", 9, map[string]any{"title": "旧"}, map[string]any{"title": "新"}); err != nil { t.Fatal(err) }
	n, err := s.CaptureSuperAuditSince(ctx, before, SuperAuditRequestMeta{Surface: "front", IPAddress: "203.0.113.10", MemberID: 2, MemberName: "家属甲", RequestMethod: "POST", RequestPath: "/matters/9"}); if err != nil || n != 1 { t.Fatalf("capture n=%d err=%v", n, err) }
	access, err := s.MemberAccessLogs(ctx, 10); if err != nil || len(access) != 1 || access[0].IPAddress != "203.0.113.10" { t.Fatalf("access=%+v err=%v", access, err) }
	if err := s.RecordFallbackSuperAudit(ctx, SuperAuditRequestMeta{Surface: "admin", IPAddress: "127.0.0.1", AdminUsername: "admin", RequestMethod: "POST", RequestPath: "/admin/test"}, "test", "update", map[string]any{"ok": true}); err != nil { t.Fatal(err) }
	adminLogs, err := s.SuperAuditLogs(ctx, "admin", 10); if err != nil || len(adminLogs) != 1 || adminLogs[0].AfterJSON == "" { t.Fatalf("admin fallback=%+v err=%v", adminLogs, err) }
	logs, err := s.SuperAuditLogs(ctx, "front", 10); if err != nil || len(logs) != 1 || logs[0].Operation != "update" || logs[0].DataCategory != "matter" || logs[0].BeforeJSON == "" || logs[0].AfterJSON == "" { t.Fatalf("logs=%+v err=%v", logs, err) }
}
