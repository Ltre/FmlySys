package store

import (
	"context"
	"database/sql"
	"mime/multipart"
	"testing"

	_ "modernc.org/sqlite"
)

func TestWorkflowEvidenceAllowsMedia(t *testing.T) {
	for _, name := range []string{"proof.mp4", "voice.mp3", "clip.webm", "audio.flac"} {
		if err := validateEvidenceHeader(&multipart.FileHeader{Filename: name, Size: 1}); err != nil {
			t.Fatalf("%s should be accepted: %v", name, err)
		}
	}
}

func TestAssetMovementsDetailedIncludesHumanLabelsAndReimbursements(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
CREATE TABLE members(id INTEGER PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE asset_events(id INTEGER PRIMARY KEY,event_type TEXT NOT NULL,amount_cent INTEGER NOT NULL,holder_member_id INTEGER NOT NULL,description TEXT NOT NULL,occurred_at TEXT NOT NULL,status TEXT NOT NULL,related_event_id INTEGER);
CREATE TABLE public_expenses(id INTEGER PRIMARY KEY,title TEXT NOT NULL,public_paid_amount_cent INTEGER NOT NULL,holder_member_id INTEGER,handler_member_id INTEGER NOT NULL,status TEXT NOT NULL,occurred_at TEXT NOT NULL);
CREATE TABLE reimbursements(id INTEGER PRIMARY KEY,expense_id INTEGER NOT NULL,payer_holder_member_id INTEGER NOT NULL,receiver_member_id INTEGER NOT NULL,amount_cent INTEGER NOT NULL,occurred_at TEXT NOT NULL,status TEXT NOT NULL);
INSERT INTO members(id,name) VALUES(1,'张三'),(2,'李四');
INSERT INTO asset_events(id,event_type,amount_cent,holder_member_id,description,occurred_at,status) VALUES
 (1,'INITIAL_ASSET',10000,1,'初始','2026-08-20T00:00:00Z','active'),
 (2,'ASSET_OUT',1000,1,'划出','2026-08-21T00:00:00Z','active');
INSERT INTO public_expenses(id,title,public_paid_amount_cent,holder_member_id,handler_member_id,status,occurred_at) VALUES
 (10,'公共采购',2000,1,1,'active','2026-08-22T00:00:00Z');
INSERT INTO reimbursements(id,expense_id,payer_holder_member_id,receiver_member_id,amount_cent,occurred_at,status) VALUES
 (20,10,1,2,300,'2026-08-23T00:00:00Z','active');
`)
	if err != nil {
		t.Fatal(err)
	}

	movements, err := New(db).AssetMovementsDetailed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{
		"初始资产": 10000,
		"资产减少": -1000,
	}
	consumerCount := 0
	for _, movement := range movements {
		if expected, ok := want[movement.Type]; ok && movement.AmountCent != expected {
			t.Fatalf("%s amount=%d want=%d", movement.Type, movement.AmountCent, expected)
		}
		if movement.Type == "消费报销" {
			consumerCount++
			if movement.AmountCent >= 0 {
				t.Fatalf("消费报销必须体现为余额减少: %+v", movement)
			}
		}
	}
	if consumerCount != 2 {
		t.Fatalf("消费报销流水=%d, want 2", consumerCount)
	}
}
