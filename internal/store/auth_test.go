package store

import "testing"

func TestNormalizePermissions(t *testing.T) {
	got, err := normalizePermissions([]string{"assets.view", "assets.view", "share.view"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(got))
	}
	if _, err := normalizePermissions([]string{"root.everything"}); err == nil {
		t.Fatal("unknown permission should fail")
	}
}
