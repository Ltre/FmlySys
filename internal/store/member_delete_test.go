package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDeleteMemberSmartHardDeletesUnusedMember(t *testing.T) {
	s := newMemberDeleteTestStore(t)
	ctx := context.Background()
	actor := insertTestMember(t, s.DB, "Dev Admin")
	target := insertTestMember(t, s.DB, "Unused")
	attachTestIdentity(t, s.DB, target)

	mode, err := s.DeleteMemberSmart(ctx, actor, target)
	if err != nil {
		t.Fatal(err)
	}
	if mode != MemberDeleteHard {
		t.Fatalf("mode=%q, want hard", mode)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM members WHERE id=?`, target).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("unused member was not physically deleted")
	}
	assertIdentityDetached(t, s.DB, target)
	assertAuthRowsGone(t, s.DB, target)
	var status string
	if err := s.DB.QueryRow(`SELECT status FROM join_requests WHERE openid='wx-target'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "draft" {
		t.Fatalf("join request status=%q, want draft", status)
	}
}

func TestDeleteMemberSmartSoftDeletesReferencedMember(t *testing.T) {
	s := newMemberDeleteTestStore(t)
	ctx := context.Background()
	actor := insertTestMember(t, s.DB, "Dev Admin")
	target := insertTestMember(t, s.DB, "Holder")
	attachTestIdentity(t, s.DB, target)
	if _, err := s.DB.Exec(`INSERT INTO asset_events(holder_member_id,created_by) VALUES(?,?)`, target, actor); err != nil {
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

	members, err := s.MembersForAccounting(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range members {
		if m.ID == target {
			found = true
			if !strings.Contains(m.Name, "（已删除）") {
				t.Fatalf("soft-deleted accounting label=%q", m.Name)
			}
		}
	}
	if !found {
		t.Fatal("soft-deleted member missing from accounting member set")
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
CREATE TABLE asset_events(id INTEGER PRIMARY KEY,holder_member_id INTEGER,created_by INTEGER,FOREIGN KEY(holder_member_id) REFERENCES members(id),FOREIGN KEY(created_by) REFERENCES members(id));
CREATE TABLE matters(id INTEGER PRIMARY KEY,owner_member_id INTEGER,created_by INTEGER,FOREIGN KEY(owner_member_id) REFERENCES members(id),FOREIGN KEY(created_by) REFERENCES members(id));
CREATE TABLE public_expenses(id INTEGER PRIMARY KEY,handler_member_id INTEGER,payer_member_id INTEGER,holder_member_id INTEGER,created_by INTEGER,FOREIGN KEY(handler_member_id) REFERENCES members(id),FOREIGN KEY(payer_member_id) REFERENCES members(id),FOREIGN KEY(holder_member_id) REFERENCES members(id),FOREIGN KEY(created_by) REFERENCES members(id));
CREATE TABLE holder_transfers(id INTEGER PRIMARY KEY,from_member_id INTEGER,to_member_id INTEGER,created_by INTEGER,FOREIGN KEY(from_member_id) REFERENCES members(id),FOREIGN KEY(to_member_id) REFERENCES members(id),FOREIGN KEY(created_by) REFERENCES members(id));
CREATE TABLE reimbursements(id INTEGER PRIMARY KEY,payer_holder_member_id INTEGER,receiver_member_id INTEGER,created_by INTEGER,FOREIGN KEY(payer_holder_member_id) REFERENCES members(id),FOREIGN KEY(receiver_member_id) REFERENCES members(id),FOREIGN KEY(created_by) REFERENCES members(id));
CREATE TABLE archives(id INTEGER PRIMARY KEY,created_by INTEGER,FOREIGN KEY(created_by) REFERENCES members(id));
CREATE TABLE attachments(id INTEGER PRIMARY KEY,uploaded_by INTEGER,FOREIGN KEY(uploaded_by) REFERENCES members(id));
CREATE TABLE record_attachments(id INTEGER PRIMARY KEY,uploaded_by INTEGER,FOREIGN KEY(uploaded_by) REFERENCES members(id));
CREATE TABLE audit_logs(id INTEGER PRIMARY KEY AUTOINCREMENT,actor_member_id INTEGER,action TEXT,entity_type TEXT,entity_id INTEGER,before_json TEXT,after_json TEXT,created_at TEXT,FOREIGN KEY(actor_member_id) REFERENCES members(id));
CREATE TABLE wechat_identities(openid TEXT PRIMARY KEY,member_id INTEGER,updated_at TEXT,FOREIGN KEY(member_id) REFERENCES members(id));
CREATE TABLE join_requests(openid TEXT PRIMARY KEY,status TEXT,rejection_reason TEXT DEFAULT '',access_token_hash TEXT DEFAULT '',access_token_expires_at TEXT DEFAULT '',requested_at TEXT DEFAULT '',reviewed_at TEXT DEFAULT '',reviewed_by TEXT DEFAULT '',updated_at TEXT);
CREATE TABLE member_permissions(member_id INTEGER,permission TEXT,FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE CASCADE);
CREATE TABLE member_sessions(token_hash TEXT PRIMARY KEY,member_id INTEGER,FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE CASCADE);
`
