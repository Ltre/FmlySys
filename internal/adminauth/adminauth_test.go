package adminauth

import (
	"testing"
	"time"
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
