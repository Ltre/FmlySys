package adminauth

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoverableBootstrapRebuildsCredentialsAndResetsUnreadableTOTP(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()
	password := "correct horse battery staple"
	if err := svc.EnsureBootstrapAdmin(ctx, "admin", password); err != nil {
		t.Fatal(err)
	}
	secret, err := svc.EnsureTOTPSecret(ctx, 1)
	if err != nil || secret == "" {
		t.Fatalf("failed to create TOTP secret: %v", err)
	}
	if _, err := db.Exec(`UPDATE admin_users SET totp_confirmed=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}

	otherKey := make([]byte, 32)
	for i := range otherKey {
		otherKey[i] = byte(200 - i)
	}
	recovered := New(db, otherKey, filepath.Dir(svc.CredentialsPath()))
	if err := recovered.EnsureBootstrapAdminRecoverable(ctx, "admin", password); err != nil {
		t.Fatal(err)
	}
	if _, err := recovered.VerifyPassword(ctx, "admin", password); err != nil {
		t.Fatalf("rebuilt credentials should verify: %v", err)
	}
	var enc string
	var confirmed, last int
	if err := db.QueryRow(`SELECT totp_secret_enc,totp_confirmed,last_totp_step FROM admin_users WHERE id=1`).Scan(&enc, &confirmed, &last); err != nil {
		t.Fatal(err)
	}
	if enc != "" || confirmed != 0 || last != -1 {
		t.Fatalf("unreadable TOTP binding was not reset: enc=%q confirmed=%d last=%d", enc, confirmed, last)
	}
}

func TestRecoverableBootstrapRequiresLocalPasswordForKeyMismatch(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()
	if err := svc.EnsureBootstrapAdmin(ctx, "admin", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	otherKey := make([]byte, 32)
	for i := range otherKey {
		otherKey[i] = byte(100 + i)
	}
	recovered := New(db, otherKey, filepath.Dir(svc.CredentialsPath()))
	err := recovered.EnsureBootstrapAdminRecoverable(ctx, "admin", "")
	if err == nil || !strings.Contains(err.Error(), "FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD") {
		t.Fatalf("expected actionable recovery error, got %v", err)
	}
}
