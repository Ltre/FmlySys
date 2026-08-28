package db_test

import (
	"database/sql"
	"testing"

	"github.com/Ltre/FmlySys/migrations"
	_ "modernc.org/sqlite"
)

func TestOwnedPermissionsAndMedicationMigration(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	defer database.Close()
	if _, err := database.Exec(`PRAGMA foreign_keys=ON;
CREATE TABLE members(id INTEGER PRIMARY KEY);
CREATE TABLE member_permissions(member_id INTEGER NOT NULL REFERENCES members(id),permission TEXT NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(member_id,permission));
INSERT INTO members(id) VALUES(1);
INSERT INTO member_permissions(member_id,permission,created_at) VALUES
(1,'matters.manage','2026-08-25T00:00:00Z'),
(1,'share.manage','2026-08-25T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	body, err := migrations.FS.ReadFile("partition/000008_owned_permissions_and_medication.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(body)); err != nil {
		t.Fatal(err)
	}

	want := []string{"matters.manage_others", "matters.manage_self", "matters.view", "share.manage_others", "share.manage_self", "share.view"}
	rows, err := database.Query(`SELECT permission FROM member_permissions ORDER BY permission`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			t.Fatal(err)
		}
		got = append(got, permission)
	}
	if len(got) != len(want) {
		t.Fatalf("migrated permissions=%v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("migrated permissions=%v", got)
		}
	}
	for _, table := range []string{"medication_plans", "medication_intake_records"} {
		var count int
		if err := database.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %s was not created", table)
		}
	}
}
