package httpserver

import "testing"

func TestNormalizeTOTPAlias(t *testing.T) {
	got, err := normalizeTOTPAlias(" FmlySys 本机测试 ", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if got != "FmlySys 本机测试" {
		t.Fatalf("unexpected alias %q", got)
	}
	got, err = normalizeTOTPAlias("", "admin")
	if err != nil || got != "admin" {
		t.Fatalf("fallback alias = %q, err=%v", got, err)
	}
}
