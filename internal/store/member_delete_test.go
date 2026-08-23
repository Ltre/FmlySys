package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDeleteMemberSmartAlwaysSoftDeletesZeroBalanceMember(t *testing.T) {
	s := newMemberDeleteTestStore(t)
	ctx := context.Background()
	actor := insertTestMember(t, s.DB, "Dev Admin")
	target := insertTestMember(t, s.DB, "Zero Balance")
	attachTestIdentity(t, s.DB, target)
	if _, err := s.DB.Exec(`INSERT INTO passkey_login_identities(id,member_id,updated_at) VALUES(1,?,'now')`, target); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO passkey_login_sessions(token_hash,identity_id) VALUES('pk-session',1)`); err != nil {
		t.Fatal(err)
	}

	mode, err := s.DeleteMemberSmart(ctx, actor, target)
	if err != nil {
		t.Fatal(err)
	}
	if mode != MemberDeleteSoft {
		t.Fatalf("mode=%q, want soft", mode)
	}
	var status string
	var isDel int
	if err := s.DB.QueryRow(`SELECT status,is_del FROM members WHERE id=?`, target).Scan(&status, &isDel); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" || isDel != 1 {
		t.Fatalf("status=%q is_del=%d", status, isDel)
	}
	assertIdentityDetached(t, s.DB, target)
	assertAuthRowsGone(t, s.DB, target)
	var linked sql.NullInt64
	if err := s.DB.QueryRow(`SELECT member_id FROM passkey_login_identities WHERE id=1`).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if linked.Valid {
		t.Fatal("passkey login identity still linked to deleted member")
	}
	var pkSessions int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM passkey_login_sessions WHERE identity_id=1`).Scan(&pkSessions); err != nil {
		t.Fatal(err)
	}
	if pkSessions != 0 {
		t.Fatal("passkey identity sessions were not revoked")
	}
}

func TestDeleteMemberSmartRejectsNonZeroHolderBalance(t *testing.T) {
	s := newMemberDeleteTestStore(t)
	actor := insertTestMember(t, s.DB, "Dev Admin")
	target := insertTestMember(t, s.DB, "Holder")
	attachTestIdentity(t, s.DB, target)
	if _, err := s.DB.Exec(`INSERT INTO asset_events(event_type,amount_cent,holder_member_id,status,created_by) VALUES('ASSET_IN',12500,?,'active',?)`, target, actor); err != nil {
		t.Fatal(err)
	}

	if _, err := s.DeleteMemberSmart(context.Background(), actor, target); err == nil || !strings.Contains(err.Error(), "只有持有资产为 0") {
		t.Fatalf("expected non-zero balance rejection, got %v", err)
	}
	var status string
	var isDel int
	if err := s.DB.QueryRow(`SELECT status,is_del FROM members WHERE id=?`, target).Scan(&status, &isDel); err != nil {
		t.Fatal(err)
	}
	if status != "active" || isDel != 0 {
		t.Fatalf("member changed despite rejection: status=%q is_del=%d", status, isDel)
	}
	var sessions int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM member_sessions WHERE member_id=?`, target).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions == 0 {
		t.Fatal("authentication state was revoked before balance validation")
	}
}

func TestDeleteMemberSmartPreservesMoneyAndAuditHistory(t *testing.T) {
	s := newMemberDeleteTestStore(t)
	ctx := context.Background()
	actor := insertTestMember(t, s.DB, "Dev Admin")
	target := insertTestMember(t, s.DB, "Historical Holder")
	attachTestIdentity(t, s.DB, target)
	if _, err := s.DB.Exec(`INSERT INTO asset_events(id,event_type,amount_cent,holder_member_id,status,created_by) VALUES(10,'ASSET_IN',10000,?,'active',?)`, target, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO public_expenses(id,handler_member_id,holder_member_id,public_paid_amount_cent,status,created_by) VALUES(20,?,?,10000,'active',?)`, target, target, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO audit_logs(actor_member_id,action,entity_type,entity_id,before_json,after_json,created_at) VALUES(?,'create','expense',20,'{}','{}','before-delete')`, target); err != nil {
		t.Fatal(err)
	}

	if _, err := s.DeleteMemberSmart(ctx, actor, target); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`SELECT COUNT(1) FROM asset_events WHERE id=10`,
		`SELECT COUNT(1) FROM public_expenses WHERE id=20`,
		`SELECT COUNT(1) FROM audit_logs WHERE actor_member_id=? AND action='create' AND entity_type='expense'`,
	} {
		var count int
		args := []any{}
		if strings.Contains(query, "?") {
			args = append(args, target)
		}
		if err := s.DB.QueryRow(query, args...).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("historical row disappeared for query %q", query)
		}
	}
	members, err := s.MembersForAccounting(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, member := range members {
		if member.ID == target {
			found = true
			if !strings.Contains(member.Name, "（已删除）") {
				t.Fatalf("deleted accounting name=%q", member.Name)
			}
		}
	}
	if !found {
		t.Fatal("deleted member missing from accounting history")
	}
}

func TestUpdateMemberInfoAudited(t *testing.T) {
	s := newMemberDeleteTestStore(t)
	actor := insertTestMember(t, s.DB, "Dev Admin")
	target := insertTestMember(t, s.DB, "Old Name")
	if err := s.UpdateMemberInfo(context.Background(), actor, target, " New Name ", " 二叔 "); err != nil {
		t.Fatal(err)
	}
	var name, relation string
	if err := s.DB.QueryRow(`SELECT name,relation FROM members WHERE id=?`, target).Scan(&name, &relation); err != nil {
		t.Fatal(err)
	}
	if name != "New Name" || relation != "二叔" {
		t.Fatalf("name=%q relation=%q", name, relation)
	}
	var auditCount int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM audit_logs WHERE action='update' AND entity_type='member' AND entity_id=?`, target).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("member update audit count=%d", auditCount)
	}
}

func TestDeleteMemberSmartRejectsAuditActor(t *testing.T) {
	s := newMemberDeleteTestStore(t)
	actor := insertTestMember(t, s.DB, "Dev Admin")
	if _, err := s.DeleteMemberSmart(context.Background(), actor, actor); err == nil {
		t.Fatal("expected system actor deletion to be rejected")
	}
}

func newMemberDeleteTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;` + memberDeleteTestSchema); err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func insertTestMember(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO members(name,relation,status,is_del,created_at) VALUES(?,'','active',0,'now')`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func attachTestIdentity(t *testing.T, db *sql.DB, memberID int64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO wechat_identities(openid,member_id,updated_at) VALUES('wx-target',?,'now')`, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO join_requests(openid,status,updated_at) VALUES('wx-target','approved','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO member_permissions(member_id,permission) VALUES(?,'assets.view')`, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO member_sessions(token_hash,member_id) VALUES('token',?)`, memberID); err != nil {
		t.Fatal(err)
	}
}

func assertIdentityDetached(t *testing.T, db *sql.DB, memberID int64) {
	t.Helper()
	var bound sql.NullInt64
	if err := db.QueryRow(`SELECT member_id FROM wechat_identities WHERE openid='wx-target'`).Scan(&bound); err != nil {
		t.Fatal(err)
	}
	if bound.Valid {
		t.Fatalf("wechat identity still bound to member %d", memberID)
	}
}

func assertAuthRowsGone(t *testing.T, db *sql.DB, memberID int64) {
	t.Helper()
	for _, query := range []string{
		`SELECT COUNT(1) FROM member_permissions WHERE member_id=?`,
		`SELECT COUNT(1) FROM member_sessions WHERE member_id=?`,
	} {
		var count int
		if err := db.QueryRow(query, memberID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("auth rows remain for member %d", memberID)
		}
	}
}

const memberDeleteTestSchema = `
CREATE TABLE members(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT,relation TEXT,status TEXT,is_del INTEGER,created_at TEXT);
CREATE TABLE asset_events(id INTEGER PRIMARY KEY,event_type TEXT,amount_cent INTEGER DEFAULT 0,holder_member_id INTEGER,status TEXT DEFAULT 'active',created_by INTEGER);
CREATE TABLE holder_transfers(id INTEGER PRIMARY KEY,from_member_id INTEGER,to_member_id INTEGER,amount_cent INTEGER DEFAULT 0,status TEXT DEFAULT 'active',created_by INTEGER);
CREATE TABLE public_expenses(id INTEGER PRIMARY KEY,handler_member_id INTEGER,payer_member_id INTEGER,holder_member_id INTEGER,public_paid_amount_cent INTEGER DEFAULT 0,status TEXT DEFAULT 'active',created_by INTEGER);
CREATE TABLE reimbursements(id INTEGER PRIMARY KEY,payer_holder_member_id INTEGER,receiver_member_id INTEGER,amount_cent INTEGER DEFAULT 0,status TEXT DEFAULT 'active',created_by INTEGER);
CREATE TABLE audit_logs(id INTEGER PRIMARY KEY AUTOINCREMENT,actor_member_id INTEGER,action TEXT,entity_type TEXT,entity_id INTEGER,before_json TEXT,after_json TEXT,created_at TEXT);
CREATE TABLE wechat_identities(openid TEXT PRIMARY KEY,member_id INTEGER,updated_at TEXT);
CREATE TABLE join_requests(openid TEXT PRIMARY KEY,status TEXT,rejection_reason TEXT DEFAULT '',access_token_hash TEXT DEFAULT '',access_token_expires_at TEXT DEFAULT '',requested_at TEXT DEFAULT '',reviewed_at TEXT DEFAULT '',reviewed_by TEXT DEFAULT '',updated_at TEXT);
CREATE TABLE member_permissions(member_id INTEGER,permission TEXT);
CREATE TABLE member_sessions(token_hash TEXT PRIMARY KEY,member_id INTEGER);
CREATE TABLE passkey_login_identities(id INTEGER PRIMARY KEY,member_id INTEGER,updated_at TEXT);
CREATE TABLE passkey_login_sessions(token_hash TEXT PRIMARY KEY,identity_id INTEGER);
CREATE TABLE passkey_login_ceremonies(token_hash TEXT PRIMARY KEY,identity_id INTEGER);
`
