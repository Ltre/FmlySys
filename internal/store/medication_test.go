package store

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newMedicationTestStore(t *testing.T) *Store {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`PRAGMA foreign_keys=ON;
CREATE TABLE members(id INTEGER PRIMARY KEY,name TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active',is_del INTEGER NOT NULL DEFAULT 0);
CREATE TABLE audit_logs(id INTEGER PRIMARY KEY AUTOINCREMENT,actor_member_id INTEGER REFERENCES members(id),action TEXT NOT NULL,entity_type TEXT NOT NULL,entity_id INTEGER,before_json TEXT,after_json TEXT,created_at TEXT NOT NULL);
CREATE TABLE medication_plans(id INTEGER PRIMARY KEY AUTOINCREMENT,patient_member_id INTEGER NOT NULL REFERENCES members(id),medicine_name TEXT NOT NULL,dosage TEXT NOT NULL,scheduled_time TEXT NOT NULL,instructions TEXT NOT NULL DEFAULT '',start_date TEXT NOT NULL,end_date TEXT,created_by INTEGER NOT NULL REFERENCES members(id),created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE medication_intake_records(id INTEGER PRIMARY KEY AUTOINCREMENT,plan_id INTEGER NOT NULL REFERENCES medication_plans(id),scheduled_date TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('taken','missed')),note TEXT NOT NULL DEFAULT '',recorded_by_member_id INTEGER NOT NULL REFERENCES members(id),recorded_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(plan_id,scheduled_date));
INSERT INTO members(id,name) VALUES(1,'老大'),(2,'家属甲');`); err != nil {
		t.Fatal(err)
	}
	return New(database)
}

func TestMedicationPlanRecordAndSummary(t *testing.T) {
	store := newMedicationTestStore(t)
	ctx := context.Background()
	if err := store.CreateMedicationPlan(ctx, 2, 1, "测试药物", "1 片", "20:30", "遵照出院医嘱", "2026-08-25"); err != nil {
		t.Fatal(err)
	}
	plans, err := store.MedicationPlansForDate(ctx, "2026-08-25")
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].PatientName != "老大" || plans[0].RecordStatus != "" {
		t.Fatalf("initial plans=%+v", plans)
	}

	if err := store.RecordMedicationIntake(ctx, 2, plans[0].ID, "2026-08-25", "taken", "当面确认"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordMedicationIntake(ctx, 1, plans[0].ID, "2026-08-25", "missed", "本人说明未服"); err != nil {
		t.Fatal(err)
	}
	plans, err = store.MedicationPlansForDate(ctx, "2026-08-25")
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].RecordStatus != "missed" || plans[0].RecordedByName != "老大" || plans[0].RecordNote != "本人说明未服" {
		t.Fatalf("updated plans=%+v", plans)
	}
	var recordCount int
	if err := store.DB.QueryRow(`SELECT COUNT(1) FROM medication_intake_records`).Scan(&recordCount); err != nil {
		t.Fatal(err)
	}
	if recordCount != 1 {
		t.Fatalf("upsert should keep one record, got %d", recordCount)
	}

	summary, err := store.MedicationSummaryThrough(ctx, "2026-08-25")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Scheduled != 1 || summary.Missed != 1 || summary.Taken != 0 || summary.Unrecorded != 0 {
		t.Fatalf("summary=%+v", summary)
	}
	if err := store.EndMedicationPlan(ctx, 2, plans[0].ID, "2026-08-25"); err != nil {
		t.Fatal(err)
	}
	nextDay, err := store.MedicationPlansForDate(ctx, "2026-08-26")
	if err != nil {
		t.Fatal(err)
	}
	if len(nextDay) != 0 {
		t.Fatalf("ended plan should not appear the next day: %+v", nextDay)
	}
	var historyCount int
	if err := store.DB.QueryRow(`SELECT COUNT(1) FROM medication_intake_records`).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 {
		t.Fatalf("ending a plan must preserve intake history, got %d", historyCount)
	}
}

func TestMedicationValidationRejectsUnverifiedValues(t *testing.T) {
	store := newMedicationTestStore(t)
	ctx := context.Background()
	if err := store.CreateMedicationPlan(ctx, 2, 1, "药物", "1片", "错误时间", "", "2026-08-25"); err == nil {
		t.Fatal("invalid schedule should fail")
	}
	if err := store.RecordMedicationIntake(ctx, 2, 99, "2026-08-25", "unknown", ""); err == nil {
		t.Fatal("invalid record status should fail")
	}
}
