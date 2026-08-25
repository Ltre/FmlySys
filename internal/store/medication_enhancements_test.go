package store

import (
	"context"
	"testing"
)

func prepareMedicationEnhancementTestSchema(t *testing.T, s *Store) {
	t.Helper()
	_, err := s.DB.Exec(`
ALTER TABLE medication_plans ADD COLUMN is_deleted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE medication_plans ADD COLUMN deleted_at TEXT;
ALTER TABLE medication_plans ADD COLUMN deleted_by INTEGER REFERENCES members(id);

CREATE TABLE medication_checkins(
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL REFERENCES medication_plans(id),
    scheduled_date TEXT NOT NULL,
    patient_member_id INTEGER NOT NULL REFERENCES members(id),
    response TEXT NOT NULL CHECK(response IN ('taken','later')),
    response_at TEXT NOT NULL,
    verification_status TEXT NOT NULL DEFAULT 'none' CHECK(verification_status IN ('none','pending','confirmed','rejected')),
    verified_by_member_id INTEGER REFERENCES members(id),
    verified_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE(plan_id,scheduled_date)
);

CREATE TABLE medication_push_subscriptions(
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER NOT NULL REFERENCES members(id),
    endpoint TEXT NOT NULL UNIQUE,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE medication_notification_deliveries(
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL REFERENCES medication_plans(id),
    scheduled_date TEXT NOT NULL,
    stage TEXT NOT NULL,
    channel TEXT NOT NULL,
    status TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_medication_delivery_auto_once
ON medication_notification_deliveries(plan_id,scheduled_date,stage,channel)
WHERE stage <> 'manual';`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMedicationEnhancedPlanCheckinVerificationAndSoftDelete(t *testing.T) {
	s := newMedicationTestStore(t)
	prepareMedicationEnhancementTestSchema(t, s)
	ctx := context.Background()

	id, err := s.CreateMedicationPlanEnhanced(
		ctx,
		2,
		1,
		"测试药",
		"1片",
		"08:30",
		"饭后",
		"2026-08-25",
		"2026-09-01",
	)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := s.MedicationPlanFullByID(ctx, id, "2026-08-25")
	if err != nil || plan.EndDate != "2026-09-01" || plan.Status("2026-08-24") != "未开始" ||
		plan.Status("2026-08-25") != "进行中" || plan.Status("2026-09-02") != "已结束" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}

	if err := s.MedicationPatientRespond(ctx, 1, id, "2026-08-25", "taken"); err != nil {
		t.Fatal(err)
	}
	needed, reason, err := s.MedicationReminderNeeded(ctx, id, "2026-08-25")
	if err != nil || needed || reason == "" {
		t.Fatalf("pending claim should pause reminders: needed=%v reason=%q err=%v", needed, reason, err)
	}

	if err := s.MedicationVerifyClaim(ctx, 2, id, "2026-08-25", false); err != nil {
		t.Fatal(err)
	}
	needed, _, err = s.MedicationReminderNeeded(ctx, id, "2026-08-25")
	if err != nil || !needed {
		t.Fatalf("rejected claim should allow later reminders: needed=%v err=%v", needed, err)
	}

	if err := s.MedicationPatientRespond(ctx, 1, id, "2026-08-25", "taken"); err != nil {
		t.Fatal(err)
	}
	if err := s.MedicationVerifyClaim(ctx, 2, id, "2026-08-25", true); err != nil {
		t.Fatal(err)
	}
	needed, _, err = s.MedicationReminderNeeded(ctx, id, "2026-08-25")
	if err != nil || needed {
		t.Fatalf("confirmed intake must stop reminders: needed=%v err=%v", needed, err)
	}

	if err := s.SoftDeleteMedicationPlan(ctx, 2, id); err != nil {
		t.Fatal(err)
	}
	plans, err := s.AllMedicationPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 0 {
		t.Fatalf("soft-deleted plan should be hidden from active plan list: %+v", plans)
	}

	var count int
	if err := s.DB.QueryRow(
		`SELECT COUNT(1) FROM medication_plans WHERE id=? AND is_deleted=1`,
		id,
	).Scan(&count); err != nil || count != 1 {
		t.Fatalf("plan must remain physically present: count=%d err=%v", count, err)
	}
}

func TestMedicationPermissionCatalogIsSplit(t *testing.T) {
	foundSelf, foundOthers, foundOld := false, false, false
	for _, p := range PermissionCatalog {
		switch p.Key {
		case "medication.manage_self":
			foundSelf = true
		case "medication.manage_others":
			foundOthers = true
		case "medication.manage":
			foundOld = true
		}
	}
	if !foundSelf || !foundOthers || foundOld {
		t.Fatalf("catalog split failed: self=%v others=%v old=%v", foundSelf, foundOthers, foundOld)
	}
	if !CanManageMedicationPlan(map[string]bool{"medication.manage_self": true}, 2, 2) {
		t.Fatal("creator should be manageable with manage_self")
	}
	if !CanManageMedicationPlan(map[string]bool{"medication.manage_others": true}, 2, 1) {
		t.Fatal("other creator should be manageable with manage_others")
	}
}
