package store

import (
	"bytes"
	"context"
	"database/sql"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func testExpenseEvidenceHeaders(t *testing.T) []*multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("evidence", "receipt.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("%PDF-1.4 receipt")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = request.MultipartForm.RemoveAll() })
	return request.MultipartForm.File["evidence"]
}

func TestExpenseEvidenceAddDeleteAndScope(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	conn.SetMaxOpenConns(1)
	_, err = conn.Exec(`
CREATE TABLE public_expenses(id INTEGER PRIMARY KEY,status TEXT NOT NULL);
CREATE TABLE record_attachments(id INTEGER PRIMARY KEY AUTOINCREMENT,entity_type TEXT,entity_id INTEGER,storage_name TEXT,original_name TEXT,mime_type TEXT,size INTEGER,sha256 TEXT,uploaded_by INTEGER,created_at TEXT);
CREATE TABLE audit_logs(id INTEGER PRIMARY KEY AUTOINCREMENT,actor_member_id INTEGER,action TEXT,entity_type TEXT,entity_id INTEGER,before_json TEXT,after_json TEXT,created_at TEXT);
INSERT INTO public_expenses(id,status) VALUES(10,'active'),(20,'active'),(30,'deleted');`)
	if err != nil {
		t.Fatal(err)
	}
	s := New(conn)
	ctx := context.Background()
	dir := t.TempDir()
	headers := testExpenseEvidenceHeaders(t)
	if err := s.AddExpenseEvidence(ctx, 1, 10, dir, headers); err != nil {
		t.Fatal(err)
	}
	attachments, err := s.EvidenceFor(ctx, "expense", 10)
	if err != nil || len(attachments) != 1 || attachments[0].OriginalName != "receipt.pdf" {
		t.Fatalf("attachments=%+v, err=%v", attachments, err)
	}
	var storage string
	if err := conn.QueryRow(`SELECT storage_name FROM record_attachments WHERE id=?`, attachments[0].ID).Scan(&storage); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, storage)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stored evidence missing: %v", err)
	}
	if err := s.DeleteExpenseEvidence(ctx, 1, 20, attachments[0].ID, dir); err == nil {
		t.Fatal("another expense must not delete this attachment")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("wrong-expense deletion changed file: %v", err)
	}
	if err := s.DeleteExpenseEvidence(ctx, 1, 10, attachments[0].ID, dir); err != nil {
		t.Fatal(err)
	}
	attachments, err = s.EvidenceFor(ctx, "expense", 10)
	if err != nil || len(attachments) != 0 {
		t.Fatalf("attachment remains after deletion: %+v, err=%v", attachments, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stored file remains after deletion: %v", err)
	}
	for _, action := range []string{"add_evidence", "delete_evidence"} {
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE entity_type='expense' AND entity_id=10 AND action=?`, action).Scan(&count); err != nil || count != 1 {
			t.Fatalf("audit %s count=%d, err=%v", action, count, err)
		}
	}
	if err := s.AddExpenseEvidence(ctx, 1, 30, dir, headers); err == nil {
		t.Fatal("inactive expense must not accept evidence")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("failed upload left files behind: %v, err=%v", files, err)
	}
}
