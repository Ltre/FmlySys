package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func newContentEditTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;
CREATE TABLE members(id INTEGER PRIMARY KEY,name TEXT NOT NULL,relation TEXT NOT NULL DEFAULT '',status TEXT NOT NULL DEFAULT 'active',is_del INTEGER NOT NULL DEFAULT 0);
CREATE TABLE matters(id INTEGER PRIMARY KEY AUTOINCREMENT,parent_id INTEGER REFERENCES matters(id),title TEXT NOT NULL,matter_type TEXT NOT NULL,description TEXT NOT NULL,status TEXT NOT NULL,start_date TEXT,due_date TEXT,owner_member_id INTEGER REFERENCES members(id),created_by INTEGER NOT NULL REFERENCES members(id),created_at TEXT NOT NULL,updated_at TEXT NOT NULL,version INTEGER NOT NULL DEFAULT 1);
CREATE TABLE public_expenses(id INTEGER PRIMARY KEY,matter_id INTEGER,amount_cent INTEGER NOT NULL DEFAULT 0,status TEXT NOT NULL DEFAULT 'active');
CREATE TABLE archives(id INTEGER PRIMARY KEY AUTOINCREMENT,title TEXT NOT NULL,category TEXT NOT NULL,content TEXT NOT NULL,visibility TEXT NOT NULL,created_by INTEGER NOT NULL REFERENCES members(id),created_at TEXT NOT NULL,updated_at TEXT NOT NULL,version INTEGER NOT NULL DEFAULT 1);
CREATE TABLE attachments(id INTEGER PRIMARY KEY AUTOINCREMENT,archive_id INTEGER NOT NULL REFERENCES archives(id) ON DELETE CASCADE,storage_name TEXT NOT NULL UNIQUE,original_name TEXT NOT NULL,mime_type TEXT NOT NULL,size INTEGER NOT NULL,sha256 TEXT NOT NULL,uploaded_by INTEGER NOT NULL REFERENCES members(id),created_at TEXT NOT NULL);
CREATE TABLE audit_logs(id INTEGER PRIMARY KEY AUTOINCREMENT,actor_member_id INTEGER REFERENCES members(id),action TEXT NOT NULL,entity_type TEXT NOT NULL,entity_id INTEGER,before_json TEXT,after_json TEXT,reason TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL);
INSERT INTO members(id,name) VALUES(1,'张三'),(2,'李四');`); err != nil {
		t.Fatal(err)
	}
	return New(db)
}

func TestUpdateMatterEditsAllFieldsAndRejectsCycles(t *testing.T) {
	s := newContentEditTestStore(t)
	ctx := context.Background()
	if err := s.CreateMatter(ctx, 1, MatterInput{Title: "父事务", OwnerMemberID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateMatter(ctx, 1, MatterInput{ParentID: 1, Title: "子事务", OwnerMemberID: 2}); err != nil {
		t.Fatal(err)
	}
	updated := MatterInput{ParentID: 1, Title: "子事务（已更新）", Type: "祖屋", Description: "完整说明", Status: "active", StartDate: "2026-08-01", DueDate: "2026-08-31", OwnerMemberID: 1}
	if err := s.UpdateMatter(ctx, 1, 2, updated); err != nil {
		t.Fatal(err)
	}
	matters, err := s.Matters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var child Matter
	for _, matter := range matters {
		if matter.ID == 2 {
			child = matter
		}
	}
	if child.Title != updated.Title || child.Type != updated.Type || child.Description != updated.Description || child.Status != updated.Status || child.StartDate != updated.StartDate || child.DueDate != updated.DueDate || child.ParentIDValue != 1 || child.OwnerMemberID != 1 {
		t.Fatalf("matter was not fully updated: %+v", child)
	}
	if err := s.UpdateMatter(ctx, 1, 1, MatterInput{ParentID: 2, Title: "父事务"}); err == nil {
		t.Fatal("expected descendant parent to be rejected")
	}
}

func TestUpdateArchiveAndDeleteAttachmentKeepsArchive(t *testing.T) {
	s := newContentEditTestStore(t)
	ctx := context.Background()
	archiveID, err := s.CreateArchive(ctx, 1, "旧标题", "旧分类", "旧正文", "family")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateFamilyArchive(ctx, 1, archiveID, "新标题", "证件资料", "新正文"); err != nil {
		t.Fatal(err)
	}
	var title, category, content string
	if err := s.DB.QueryRow(`SELECT title,category,content FROM archives WHERE id=?`, archiveID).Scan(&title, &category, &content); err != nil {
		t.Fatal(err)
	}
	if title != "新标题" || category != "证件资料" || content != "新正文" {
		t.Fatalf("archive fields=%q/%q/%q", title, category, content)
	}
	uploadDir := t.TempDir()
	storageName := "proof.txt"
	if err := os.WriteFile(filepath.Join(uploadDir, storageName), []byte("proof"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := s.DB.Exec(`INSERT INTO attachments(archive_id,storage_name,original_name,mime_type,size,sha256,uploaded_by,created_at) VALUES(?,?,?,?,?,?,?,?)`, archiveID, storageName, "凭证.txt", "text/plain", 5, "hash", 1, now())
	if err != nil {
		t.Fatal(err)
	}
	attachmentID, _ := res.LastInsertId()
	if err := s.DeleteFamilyArchiveAttachment(ctx, 1, archiveID, attachmentID, uploadDir); err != nil {
		t.Fatal(err)
	}
	var archiveCount, attachmentCount int
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM archives WHERE id=?`, archiveID).Scan(&archiveCount)
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM attachments WHERE id=?`, attachmentID).Scan(&attachmentCount)
	if archiveCount != 1 || attachmentCount != 0 {
		t.Fatalf("archiveCount=%d attachmentCount=%d", archiveCount, attachmentCount)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, storageName)); !os.IsNotExist(err) {
		t.Fatalf("attachment file still exists or stat failed unexpectedly: %v", err)
	}
}
