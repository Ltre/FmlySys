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

func TestNormalizePermissionsAddsViewDependencies(t *testing.T) {
	got, err := normalizePermissions([]string{"matters.manage_others", "share.manage_self", "medication.manage"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"matters.manage_others": true,
		"matters.view":          true,
		"share.manage_self":     true,
		"share.view":            true,
		"medication.manage":     true,
		"medication.view":       true,
	}
	if len(got) != len(want) {
		t.Fatalf("permissions=%v", got)
	}
	for _, permission := range got {
		if !want[permission] {
			t.Fatalf("unexpected permission %q in %v", permission, got)
		}
	}
}

func TestCanManageCreatedRecordSeparatesSelfAndOthers(t *testing.T) {
	if !CanManageCreatedRecord(map[string]bool{"share.manage_self": true}, 7, 7, "share") {
		t.Fatal("self permission should manage a self-created share")
	}
	if CanManageCreatedRecord(map[string]bool{"share.manage_self": true}, 7, 8, "share") {
		t.Fatal("self permission must not manage another member's share")
	}
	if !CanManageCreatedRecord(map[string]bool{"matters.manage_others": true}, 7, 8, "matters") {
		t.Fatal("others permission should manage another member's matter")
	}
	if CanManageCreatedRecord(map[string]bool{"matters.manage_others": true}, 7, 7, "matters") {
		t.Fatal("others permission must not implicitly manage a self-created matter")
	}
	if CanManageCreatedRecord(map[string]bool{"share.manage": true}, 7, 8, "share") {
		t.Fatal("removed broad permission must not bypass the split boundary")
	}
	if CanManageCreatedRecord(map[string]bool{"share.manage_self": true}, 7, 7, "unknown") {
		t.Fatal("unknown record domain must be rejected")
	}
}
