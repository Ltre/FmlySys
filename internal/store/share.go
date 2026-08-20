package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) CreateArchive(ctx context.Context, actor int64, title, category, content, visibility string) (int64, error) {
	if title == "" {
		return 0, errors.New("资料标题不能为空")
	}
	if visibility != "family" && visibility != "admin" {
		visibility = "family"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO archives(title,category,content,visibility,created_by,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,1)`, title, category, content, visibility, actor, now(), now())
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "archive", id, nil, map[string]any{"title": title, "category": category, "visibility": visibility}); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) Archives(ctx context.Context) ([]Archive, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,title,category,content,visibility,created_at FROM archives ORDER BY created_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Archive
	for rows.Next() {
		var v Archive
		if err := rows.Scan(&v.ID, &v.Title, &v.Category, &v.Content, &v.Visibility, &v.CreatedAt); err != nil {
			return nil, err
		}
		atts, err := s.Attachments(ctx, v.ID)
		if err != nil {
			return nil, err
		}
		v.Attachments = atts
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Attachments(ctx context.Context, archiveID int64) ([]Attachment, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,original_name,mime_type,size FROM attachments WHERE archive_id=? ORDER BY id`, archiveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attachment
	for rows.Next() {
		var v Attachment
		if err := rows.Scan(&v.ID, &v.OriginalName, &v.MimeType, &v.Size); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SaveArchiveAttachment(ctx context.Context, actor, archiveID int64, uploadDir string, header *multipart.FileHeader) error {
	if header == nil || header.Size <= 0 {
		return errors.New("附件为空")
	}
	if header.Size > 50<<20 {
		return errors.New("单个附件暂限 50MB")
	}
	src, err := header.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(uploadDir, "upload-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(src, (50<<20)+1))
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n > 50<<20 {
		return errors.New("单个附件暂限 50MB")
	}
	hash := hex.EncodeToString(h.Sum(nil))
	storage := hash[:20] + "-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if ext := sanitizeExt(filepath.Ext(header.Filename)); ext != "" {
		storage += "." + ext
	}
	final := filepath.Join(uploadDir, storage)
	if _, err := os.Stat(final); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(tmpName, final); err != nil {
			return err
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO attachments(archive_id,storage_name,original_name,mime_type,size,sha256,uploaded_by,created_at) VALUES(?,?,?,?,?,?,?,?)`, archiveID, storage, filepath.Base(header.Filename), header.Header.Get("Content-Type"), n, hash, actor, now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "attachment", id, nil, map[string]any{"archive_id": archiveID, "original_name": filepath.Base(header.Filename), "sha256": hash}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AttachmentPath(ctx context.Context, id int64, uploadDir string) (string, string, error) {
	var storage, original string
	if err := s.DB.QueryRowContext(ctx, `SELECT storage_name,original_name FROM attachments WHERE id=?`, id).Scan(&storage, &original); err != nil {
		return "", "", err
	}
	return filepath.Join(uploadDir, storage), original, nil
}

func sanitizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	var b strings.Builder
	for _, r := range ext {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
