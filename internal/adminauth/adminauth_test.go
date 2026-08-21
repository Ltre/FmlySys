package adminauth

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestTOTPAtRFC6238SHA1(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	got, err := TOTPAt(secret, 59/30, 8)
	if err != nil {
		t.Fatal(err)
	}
	if got != "94287082" {
		t.Fatalf("got %s", got)
	}
}

func TestValidateTOTPRejectsReplay(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	at := time.Unix(59, 0)
	code, _ := TOTPAt(secret, at.Unix()/30, 6)
	step, ok := ValidateTOTP(secret, code, at, -1)
	if !ok {
		t.Fatal("expected valid code")
	}
	if _, ok := ValidateTOTP(secret, code, at, step); ok {
		t.Fatal("replayed code should be rejected")
	}
}

func TestPasswordHash(t *testing.T) {
	h, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(h, "correct horse battery staple") || verifyPassword(h, "wrong") {
		t.Fatal("password verification mismatch")
	}
}

func TestEncryptedCredentialsKeepPasswordOutOfDatabase(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()
	password := "correct horse battery staple"
	if err := svc.EnsureBootstrapAdmin(ctx, "admin", password); err != nil {
		t.Fatal(err)
	}

	var dbHash string
	if err := db.QueryRow(`SELECT password_hash FROM admin_users WHERE username='admin'`).Scan(&dbHash); err != nil {
		t.Fatal(err)
	}
	if dbHash != "" {
		t.Fatalf("database must not retain password hash, got %q", dbHash)
	}
	if _, err := svc.VerifyPassword(ctx, "admin", password); err != nil {
		t.Fatalf("encrypted credential should verify: %v", err)
	}
	if _, err := svc.VerifyPassword(ctx, "admin", "wrong password"); err == nil {
		t.Fatal("wrong password unexpectedly verified")
	}

	b, err := os.ReadFile(svc.CredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := string(b)
	if strings.Contains(ciphertext, password) || strings.Contains(ciphertext, "pbkdf2-sha256") || strings.Contains(ciphertext, `"password_hash"`) {
		t.Fatal("credential file leaked plaintext credential material")
	}
}

func TestConfiguredPasswordResetsEncryptedCredentialsWithoutDeletingDatabase(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	oldPassword := "first password 123"
	newPassword := "second password 456"
	if err := svc.EnsureBootstrapAdmin(ctx, "admin", oldPassword); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureBootstrapAdmin(ctx, "admin", newPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyPassword(ctx, "admin", oldPassword); err == nil {
		t.Fatal("old password should stop working after local reset")
	}
	if _, err := svc.VerifyPassword(ctx, "admin", newPassword); err != nil {
		t.Fatalf("new password should verify after local reset: %v", err)
	}
}

func TestLegacyDatabaseHashMigratesToEncryptedCredentials(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()
	password := "legacy password 123"
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO admin_users(username,password_hash,totp_confirmed,last_totp_step,status,created_at,updated_at) VALUES(?,?,0,-1,'active',?,?)`, "admin", hash, now(), now()); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureBootstrapAdmin(ctx, "admin", ""); err != nil {
		t.Fatal(err)
	}
	var dbHash string
	if err := db.QueryRow(`SELECT password_hash FROM admin_users WHERE username='admin'`).Scan(&dbHash); err != nil {
		t.Fatal(err)
	}
	if dbHash != "" {
		t.Fatal("legacy database password hash was not cleared")
	}
	if _, err := svc.VerifyPassword(ctx, "admin", password); err != nil {
		t.Fatalf("migrated password should still verify: %v", err)
	}
}

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
CREATE TABLE admin_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret_enc TEXT NOT NULL DEFAULT '',
    totp_confirmed INTEGER NOT NULL DEFAULT 0,
    last_totp_step INTEGER NOT NULL DEFAULT -1,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE admin_sessions (
    token_hash TEXT PRIMARY KEY,
    admin_user_id INTEGER NOT NULL,
    stage TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);`)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return New(db, key, t.TempDir()), db
}
