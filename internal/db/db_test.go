package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAllowsReentrantReadWhileRowsRemainOpen(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "reentrant.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`CREATE TABLE items(id INTEGER PRIMARY KEY); INSERT INTO items(id) VALUES(1);`); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(`SELECT id FROM items`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected outer row")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(1) FROM items`).Scan(&count); err != nil {
		t.Fatalf("nested read waited on outer rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("unexpected count %d", count)
	}
}
