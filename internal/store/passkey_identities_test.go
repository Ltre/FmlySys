package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	_ "modernc.org/sqlite"
)

func TestNormalizePasskeyPhone(t *testing.T) {
	cases := map[string]string{
		"13800138000":       "+8613800138000",
		"+86 138-0013-8000": "+8613800138000",
		"0086 138 0013 8000": "+8613800138000",
		"+65 (8123) 4567":   "+6581234567",
		"1234567":            "1234567",
	}
	for input, want := range cases {
		got, err := normalizePasskeyPhone(input)
		if err != nil {
			t.Fatalf("normalize %q: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalize %q=%q, want %q", input, got, want)
		}
	}
	for _, input := range []string{"", "123", "138abc8000"} {
		if _, err := normalizePasskeyPhone(input); err == nil {
			t.Fatalf("expected invalid phone %q", input)
		}
	}
}

func TestPasskeyLoginIdentityLifecycle(t *testing.T) {
	s := newPasskeyIdentityTestStore(t)
	ctx := context.Background()
	candidate, err := s.NewPasskeyLoginCandidate(ctx, "13800138000", "张三 / iPhone")
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Phone != "+8613800138000" || len(candidate.UserHandle) != 32 {
		t.Fatalf("candidate=%+v handle=%d", candidate, len(candidate.UserHandle))
	}

	credential := &webauthn.Credential{
		ID:              []byte{1, 2, 3, 4},
		PublicKey:       []byte{5, 6, 7},
		AttestationType: "none",
		Flags:           webauthn.NewCredentialFlags(protocol.FlagUserPresent | protocol.FlagUserVerified),
		Authenticator:   webauthn.Authenticator{SignCount: 7},
	}
	identityID, err := s.CreatePasskeyLoginIdentity(ctx, candidate.Phone, candidate.DisplayName, "fmly.miku.us", candidate.UserHandle, credential)
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.PasskeyLoginUserByPhone(ctx, "+86 138 0013 8000", "fmly.miku.us")
	if err != nil {
		t.Fatal(err)
	}
	if user.IdentityID != identityID || len(user.Credentials) != 1 || string(user.UserHandle) != string(candidate.UserHandle) {
		t.Fatalf("user=%+v", user)
	}
	if _, err := s.NewPasskeyLoginCandidate(ctx, "13800138000", "重复"); err == nil {
		t.Fatal("duplicate phone should not create a new identity")
	}

	sessionToken, err := s.CreatePasskeyLoginIdentitySession(ctx, identityID)
	if err != nil {
		t.Fatal(err)
	}
	view, fresh, err := s.PasskeyLoginIdentityFromSession(ctx, sessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh || view.Phone != "+8613800138000" || len(view.Credentials) != 1 {
		t.Fatalf("view=%+v fresh=%v", view, fresh)
	}

	memberID := insertPasskeyIdentityTestMember(t, s.DB, "张三")
	if err := s.BindPasskeyLoginIdentity(ctx, identityID, memberID); err != nil {
		t.Fatal(err)
	}
	view, _, err = s.PasskeyLoginIdentityFromSession(ctx, sessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if view.MemberID != memberID || view.MemberName != "张三" {
		t.Fatalf("bound view=%+v", view)
	}
}

func TestPasskeyLoginCeremonyIsSingleUse(t *testing.T) {
	s := newPasskeyIdentityTestStore(t)
	ctx := context.Background()
	session := &webauthn.SessionData{
		Challenge:        "0123456789abcdef0123456789abcdef",
		RelyingPartyID:   "fmly.miku.us",
		UserID:           make([]byte, 32),
		UserVerification: protocol.VerificationRequired,
	}
	token, err := s.CreatePasskeyLoginCeremony(ctx, "create", 0, "fmly.miku.us", "13800138000", "张三 / iPhone", session)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.TakePasskeyLoginCeremony(ctx, token, "create", "fmly.miku.us")
	if err != nil {
		t.Fatal(err)
	}
	if got.Phone != "+8613800138000" || got.Remark != "张三 / iPhone" {
		t.Fatalf("ceremony=%+v", got)
	}
	if _, err := s.TakePasskeyLoginCeremony(ctx, token, "create", "fmly.miku.us"); err == nil {
		t.Fatal("ceremony token was reusable")
	}
}

func newPasskeyIdentityTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;` + passkeyIdentityTestSchema); err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func insertPasskeyIdentityTestMember(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO members(name,relation,status,is_del) VALUES(?,'','active',0)`, name)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

const passkeyIdentityTestSchema = `
CREATE TABLE members(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,relation TEXT NOT NULL DEFAULT '',status TEXT NOT NULL,is_del INTEGER NOT NULL DEFAULT 0);
CREATE TABLE passkey_login_identities(id INTEGER PRIMARY KEY AUTOINCREMENT,phone TEXT NOT NULL UNIQUE,profile_remark TEXT NOT NULL,member_id INTEGER REFERENCES members(id) ON DELETE CASCADE,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE passkey_login_users(identity_id INTEGER NOT NULL REFERENCES passkey_login_identities(id) ON DELETE CASCADE,rp_id TEXT NOT NULL,user_handle BLOB NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(identity_id,rp_id),UNIQUE(rp_id,user_handle));
CREATE TABLE passkey_login_credentials(id INTEGER PRIMARY KEY AUTOINCREMENT,identity_id INTEGER NOT NULL REFERENCES passkey_login_identities(id) ON DELETE CASCADE,rp_id TEXT NOT NULL,credential_id BLOB NOT NULL,credential_json TEXT NOT NULL,flags INTEGER NOT NULL DEFAULT 0,remark TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,last_used_at TEXT NOT NULL DEFAULT '',UNIQUE(rp_id,credential_id),FOREIGN KEY(identity_id,rp_id) REFERENCES passkey_login_users(identity_id,rp_id) ON DELETE CASCADE);
CREATE TABLE passkey_login_ceremonies(token_hash TEXT PRIMARY KEY,kind TEXT NOT NULL,identity_id INTEGER REFERENCES passkey_login_identities(id) ON DELETE CASCADE,rp_id TEXT NOT NULL,phone TEXT NOT NULL DEFAULT '',remark TEXT NOT NULL DEFAULT '',session_json TEXT NOT NULL,expires_at TEXT NOT NULL,created_at TEXT NOT NULL);
CREATE TABLE passkey_login_sessions(token_hash TEXT PRIMARY KEY,identity_id INTEGER NOT NULL REFERENCES passkey_login_identities(id) ON DELETE CASCADE,expires_at TEXT NOT NULL,verified_at TEXT NOT NULL,created_at TEXT NOT NULL,last_seen_at TEXT NOT NULL);
`
