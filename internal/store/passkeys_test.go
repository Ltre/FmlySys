package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	_ "modernc.org/sqlite"
)

func TestNormalizePasskeyRemark(t *testing.T) {
	got, err := normalizePasskeyRemark("  张三 / 138****1234 / iPhone 16  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "张三 / 138****1234 / iPhone 16" {
		t.Fatalf("remark=%q", got)
	}
	for _, bad := range []string{"", "   ", "张三\n13800138000"} {
		if _, err := normalizePasskeyRemark(bad); err == nil {
			t.Fatalf("expected invalid remark %q", bad)
		}
	}
}

func TestPasskeyUserCredentialAndCeremonyLifecycle(t *testing.T) {
	s := newPasskeyTestStore(t)
	ctx := context.Background()
	memberID := insertPasskeyTestMember(t, s.DB, "张三")

	user, err := s.PasskeyUserForMember(ctx, memberID, "fmly.miku.us", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(user.UserHandle) != 32 {
		t.Fatalf("user handle length=%d, want 32", len(user.UserHandle))
	}
	again, err := s.PasskeyUserForMember(ctx, memberID, "fmly.miku.us", true)
	if err != nil {
		t.Fatal(err)
	}
	if string(user.UserHandle) != string(again.UserHandle) {
		t.Fatal("user handle changed between calls")
	}

	credential := &webauthn.Credential{
		ID:              []byte{1, 2, 3, 4},
		PublicKey:       []byte{5, 6, 7},
		AttestationType: "none",
		Flags:           webauthn.NewCredentialFlags(protocol.FlagUserPresent | protocol.FlagUserVerified),
		Authenticator:   webauthn.Authenticator{SignCount: 7},
	}
	if err := s.SavePasskeyCredential(ctx, memberID, "fmly.miku.us", "张三 / iPhone", credential); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.PasskeyUserForMember(ctx, memberID, "fmly.miku.us", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Credentials) != 1 || loaded.Credentials[0].Authenticator.SignCount != 7 {
		t.Fatalf("credentials=%+v", loaded.Credentials)
	}
	views, err := s.MemberPasskeys(ctx, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Remark != "张三 / iPhone" {
		t.Fatalf("views=%+v", views)
	}

	session := &webauthn.SessionData{
		Challenge:        "0123456789abcdef0123456789abcdef",
		RelyingPartyID:   "fmly.miku.us",
		UserID:           user.UserHandle,
		UserVerification: protocol.VerificationRequired,
	}
	token, err := s.CreatePasskeyCeremony(ctx, "register", memberID, "fmly.miku.us", "张三 / iPhone", session)
	if err != nil {
		t.Fatal(err)
	}
	ceremony, err := s.TakePasskeyCeremony(ctx, token, "register", "fmly.miku.us")
	if err != nil {
		t.Fatal(err)
	}
	if ceremony.MemberID != memberID || ceremony.Remark != "张三 / iPhone" || ceremony.Session.Challenge != session.Challenge {
		t.Fatalf("ceremony=%+v", ceremony)
	}
	if _, err := s.TakePasskeyCeremony(ctx, token, "register", "fmly.miku.us"); err == nil {
		t.Fatal("ceremony token was reusable")
	}

	if err := s.DeleteOwnPasskey(ctx, memberID, views[0].ID); err != nil {
		t.Fatal(err)
	}
	views, err = s.MemberPasskeys(ctx, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatalf("passkey still exists: %+v", views)
	}
}

func newPasskeyTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;` + passkeyTestSchema); err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func insertPasskeyTestMember(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO members(name,status,is_del) VALUES(?,'active',0)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

const passkeyTestSchema = `
CREATE TABLE members(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,status TEXT NOT NULL,is_del INTEGER NOT NULL DEFAULT 0);
CREATE TABLE passkey_users(member_id INTEGER NOT NULL,rp_id TEXT NOT NULL,user_handle BLOB NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(member_id,rp_id),UNIQUE(rp_id,user_handle),FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE CASCADE);
CREATE TABLE passkey_credentials(id INTEGER PRIMARY KEY AUTOINCREMENT,member_id INTEGER NOT NULL,rp_id TEXT NOT NULL,credential_id BLOB NOT NULL,credential_json TEXT NOT NULL,flags INTEGER NOT NULL DEFAULT 0,remark TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,last_used_at TEXT NOT NULL DEFAULT '',UNIQUE(rp_id,credential_id),FOREIGN KEY(member_id,rp_id) REFERENCES passkey_users(member_id,rp_id) ON DELETE CASCADE);
CREATE TABLE passkey_ceremonies(token_hash TEXT PRIMARY KEY,kind TEXT NOT NULL,member_id INTEGER,rp_id TEXT NOT NULL,session_json TEXT NOT NULL,remark TEXT NOT NULL DEFAULT '',expires_at TEXT NOT NULL,created_at TEXT NOT NULL,FOREIGN KEY(member_id) REFERENCES members(id) ON DELETE CASCADE);
`
