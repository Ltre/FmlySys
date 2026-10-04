package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newWeChatCodeLoginTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
CREATE TABLE wechat_code_login_attempts (
 state_hash TEXT PRIMARY KEY, scene_hash TEXT NOT NULL UNIQUE, openid TEXT NOT NULL DEFAULT '',
 code_hash TEXT NOT NULL DEFAULT '', expires_at TEXT NOT NULL, code_expires_at TEXT NOT NULL DEFAULT '',
 code_issued_at TEXT NOT NULL DEFAULT '', scanned_at TEXT NOT NULL DEFAULT '',
 failed_attempts INTEGER NOT NULL DEFAULT 0, consumed_at TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL
);
CREATE TABLE members(id INTEGER PRIMARY KEY, name TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'active');
CREATE TABLE wechat_identities(openid TEXT PRIMARY KEY, unionid TEXT NOT NULL DEFAULT '', member_id INTEGER);
`)
	if err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func TestWeChatCodeLoginAttemptIsBrowserBoundAndOneTime(t *testing.T) {
	ctx := context.Background()
	s := newWeChatCodeLoginTestStore(t)
	now := time.Now().UTC()
	if err := s.CreateWeChatCodeLoginAttempt(ctx, "state-hash", "scene-hash", now.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if claimed, err := s.MarkWeChatCodeLoginScanned(ctx, "scene-hash", "oa-openid", now); err != nil || !claimed {
		t.Fatalf("first scan claimed=%v err=%v", claimed, err)
	}
	if claimed, err := s.MarkWeChatCodeLoginScanned(ctx, "scene-hash", "other-openid", now); err != nil || claimed {
		t.Fatalf("second identity must not claim the QR, claimed=%v err=%v", claimed, err)
	}
	if err := s.IssueWeChatCodeLoginCode(ctx, "oa-openid", "hmac-code", now.Add(time.Second), now.Add(5*time.Minute), 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := s.IssueWeChatCodeLoginCode(ctx, "oa-openid", "another-hash", now.Add(2*time.Second), now.Add(5*time.Minute), 30*time.Second); !errors.Is(err, ErrWeChatLoginCodeRateLimit) {
		t.Fatalf("expected same-attempt cooldown, got %v", err)
	}
	if _, err := s.ConsumeWeChatCodeLoginCode(ctx, "other-state", "hmac-code", now.Add(3*time.Second)); !errors.Is(err, ErrInvalidWeChatLoginAttempt) {
		t.Fatalf("expected wrong browser state rejection, got %v", err)
	}
	openID, err := s.ConsumeWeChatCodeLoginCode(ctx, "state-hash", "hmac-code", now.Add(3*time.Second))
	if err != nil || openID != "oa-openid" {
		t.Fatalf("consumed openID=%q err=%v", openID, err)
	}
	if _, err := s.ConsumeWeChatCodeLoginCode(ctx, "state-hash", "hmac-code", now.Add(4*time.Second)); !errors.Is(err, ErrInvalidWeChatLoginAttempt) {
		t.Fatalf("expected one-time code rejection, got %v", err)
	}
}

func TestWeChatCodeLoginRejectsFiveWrongCodesAndExpiredAttempts(t *testing.T) {
	ctx := context.Background()
	s := newWeChatCodeLoginTestStore(t)
	now := time.Now().UTC()
	if err := s.CreateWeChatCodeLoginAttempt(ctx, "state-hash", "scene-hash", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if claimed, err := s.MarkWeChatCodeLoginScanned(ctx, "scene-hash", "oa-openid", now); err != nil || !claimed {
		t.Fatalf("scan claimed=%v err=%v", claimed, err)
	}
	if err := s.IssueWeChatCodeLoginCode(ctx, "oa-openid", "expected-hash", now, now.Add(time.Minute), time.Second); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := s.ConsumeWeChatCodeLoginCode(ctx, "state-hash", "wrong-hash", now.Add(time.Second)); !errors.Is(err, ErrInvalidWeChatLoginAttempt) {
			t.Fatalf("attempt %d returned %v", attempt+1, err)
		}
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM wechat_code_login_attempts WHERE state_hash='state-hash'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("locked attempt count=%d err=%v", count, err)
	}
	if err := s.CreateWeChatCodeLoginAttempt(ctx, "expired-state", "expired-scene", now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if claimed, err := s.MarkWeChatCodeLoginScanned(ctx, "expired-scene", "oa-openid", now); err != nil || claimed {
		t.Fatalf("expired scan claimed=%v err=%v", claimed, err)
	}
}

func TestBoundMemberForWeChatLoginUsesUniqueUnionID(t *testing.T) {
	ctx := context.Background()
	s := newWeChatCodeLoginTestStore(t)
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO members(id,name) VALUES(1,'Member A'),(2,'Member B');
INSERT INTO wechat_identities(openid,unionid,member_id) VALUES('web-openid','shared-union',1),('oa-openid','',NULL)`); err != nil {
		t.Fatal(err)
	}
	memberID, err := s.BoundMemberForWeChatLogin(ctx, "oa-openid", "shared-union")
	if err != nil || memberID != 1 {
		t.Fatalf("union-bound member=%d err=%v", memberID, err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO wechat_identities(openid,unionid,member_id) VALUES('other-openid','shared-union',2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BoundMemberForWeChatLogin(ctx, "oa-openid", "shared-union"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ambiguous UnionID should not grant access, got %v", err)
	}
}
