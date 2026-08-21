package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Several read paths intentionally load child records while parent rows are
	// still being scanned. A single database/sql connection self-deadlocks in
	// that pattern because the nested read waits for the connection held by the
	// outer rows. Keep a small bounded pool: WAL allows concurrent readers while
	// SQLite + busy_timeout continue to serialize conflicting writers.
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(4)
	conn.SetConnMaxIdleTime(5 * time.Minute)
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func Migrate(ctx context.Context, conn *sql.DB, files fs.FS, dir string) error {
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var exists int
		err := conn.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, name).Scan(&exists)
		if err != nil {
			return err
		}
		if exists != 0 {
			continue
		}
		body, err := fs.ReadFile(files, dir+"/"+name)
		if err != nil {
			return err
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
