package partition

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Ltre/FmlySys/internal/db"
	"github.com/Ltre/FmlySys/migrations"
)

type Manager struct {
	DataDir   string
	SystemDB  *sql.DB
	ActiveID  string
	ActiveDir string
	ActiveDB  *sql.DB
}

func Open(ctx context.Context, dataDir string) (*Manager, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "partitions"), 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "temp"), 0o750); err != nil {
		return nil, err
	}
	systemDB, err := db.Open(filepath.Join(dataDir, "system.db"))
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, systemDB, migrations.FS, "system"); err != nil {
		systemDB.Close()
		return nil, err
	}

	m := &Manager{DataDir: dataDir, SystemDB: systemDB}
	if err := m.ensureDefault(ctx); err != nil {
		m.Close()
		return nil, err
	}
	if err := m.openActive(ctx); err != nil {
		m.Close()
		return nil, err
	}
	return m, nil
}

func (m *Manager) ensureDefault(ctx context.Context) error {
	var count int
	if err := m.SystemDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM data_partitions`).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	id := "p_default"
	rel := filepath.ToSlash(filepath.Join("partitions", id))
	dir := filepath.Join(m.DataDir, rel)
	if err := os.MkdirAll(filepath.Join(dir, "uploads"), 0o750); err != nil {
		return err
	}
	_, err := m.SystemDB.ExecContext(ctx, `INSERT INTO data_partitions(id, display_name, path, source_type, created_at, is_active) VALUES(?, ?, ?, 'INITIAL', ?, 1)`, id, "默认数据分区", rel, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (m *Manager) openActive(ctx context.Context) error {
	var id, rel string
	if err := m.SystemDB.QueryRowContext(ctx, `SELECT id, path FROM data_partitions WHERE is_active = 1 LIMIT 1`).Scan(&id, &rel); err != nil {
		return fmt.Errorf("active partition: %w", err)
	}
	dir := filepath.Join(m.DataDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Join(dir, "uploads"), 0o750); err != nil {
		return err
	}
	activeDB, err := db.Open(filepath.Join(dir, "fmlysys.db"))
	if err != nil {
		return err
	}
	if err := db.Migrate(ctx, activeDB, migrations.FS, "partition"); err != nil {
		activeDB.Close()
		return err
	}
	m.ActiveID = id
	m.ActiveDir = dir
	m.ActiveDB = activeDB
	_, _ = m.SystemDB.ExecContext(ctx, `UPDATE data_partitions SET last_opened_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), id)
	return nil
}

func (m *Manager) Close() error {
	if m.ActiveDB != nil {
		_ = m.ActiveDB.Close()
	}
	if m.SystemDB != nil {
		return m.SystemDB.Close()
	}
	return nil
}
