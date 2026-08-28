package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func normalizeArchiveFields(title, category, content string) (string, string, string, error) {
	title = strings.TrimSpace(title)
	category = strings.TrimSpace(category)
	content = strings.TrimSpace(content)
	if title == "" {
		return "", "", "", errors.New("资料标题不能为空")
	}
	if utf8.RuneCountInString(title) > 160 {
		return "", "", "", errors.New("资料标题最多 160 个字符")
	}
	if category == "" {
		category = "其他"
	}
	if utf8.RuneCountInString(category) > 80 {
		return "", "", "", errors.New("资料分类最多 80 个字符")
	}
	return title, category, content, nil
}

// UTF8Summary truncates by Unicode code points rather than bytes, so Chinese
// text and other multi-byte UTF-8 characters cannot be split into invalid data.
func UTF8Summary(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func (s *Store) CreateArchive(ctx context.Context, actor int64, title, category, content, visibility string) (int64, error) {
	var err error
	title, category, content, err = normalizeArchiveFields(title, category, content)
	if err != nil {
		return 0, err
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

func (s *Store) UpdateFamilyArchive(ctx context.Context, actor, archiveID int64, title, category, content string) error {
	if archiveID <= 0 {
		return errors.New("共享资料不存在")
	}
	var err error
	title, category, content, err = normalizeArchiveFields(title, category, content)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldTitle, oldCategory, oldContent string
	if err := tx.QueryRowContext(ctx, `SELECT title,category,content FROM archives WHERE id=? AND visibility='family'`, archiveID).Scan(&oldTitle, &oldCategory, &oldContent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("共享资料不存在")
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE archives SET title=?,category=?,content=?,updated_at=?,version=version+1 WHERE id=? AND visibility='family'`, title, category, content, now(), archiveID); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "update", "archive", archiveID,
		map[string]any{"title": oldTitle, "category": oldCategory, "content": oldContent, "visibility": "family"},
		map[string]any{"title": title, "category": category, "content": content, "visibility": "family"}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteFamilyArchiveAttachment(ctx context.Context, actor, archiveID, attachmentID int64, uploadDir string) error {
	if archiveID <= 0 || attachmentID <= 0 {
		return errors.New("附件不存在")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var storage, original string
	if err := tx.QueryRowContext(ctx, `
SELECT a.storage_name,a.original_name
FROM attachments a
JOIN archives ar ON ar.id=a.archive_id
WHERE a.id=? AND a.archive_id=? AND ar.visibility='family'`, attachmentID, archiveID).Scan(&storage, &original); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("附件不存在")
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM attachments WHERE id=? AND archive_id=?`, attachmentID, archiveID); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "delete", "attachment", attachmentID,
		map[string]any{"archive_id": archiveID, "original_name": original}, nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(uploadDir, storage)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("附件记录已删除，但存储文件清理失败: %w", err)
	}
	return nil
}

func (s *Store) Archives(ctx context.Context) ([]Archive, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT a.id,a.title,a.category,a.content,a.visibility,a.created_by,COALESCE(m.name,''),a.created_at,
       (SELECT COUNT(1) FROM attachments att WHERE att.archive_id=a.id)
FROM archives a
LEFT JOIN members m ON m.id=a.created_by
ORDER BY a.created_at DESC,a.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Archive
	for rows.Next() {
		var v Archive
		if err := rows.Scan(&v.ID, &v.Title, &v.Category, &v.Content, &v.Visibility, &v.CreatedBy, &v.CreatorName, &v.CreatedAt, &v.AttachmentCount); err != nil {
			return nil, err
		}
		v.Summary = UTF8Summary(v.Content, 100)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) FamilyArchiveByID(ctx context.Context, id int64) (Archive, error) {
	var archive Archive
	err := s.DB.QueryRowContext(ctx, `
SELECT a.id,a.title,a.category,a.content,a.visibility,a.created_by,COALESCE(m.name,''),a.created_at
FROM archives a
LEFT JOIN members m ON m.id=a.created_by
WHERE a.id=? AND a.visibility='family'`, id).Scan(
		&archive.ID, &archive.Title, &archive.Category, &archive.Content, &archive.Visibility,
		&archive.CreatedBy, &archive.CreatorName, &archive.CreatedAt,
	)
	if err != nil {
		return Archive{}, err
	}
	archive.Attachments, err = s.Attachments(ctx, archive.ID)
	archive.AttachmentCount = len(archive.Attachments)
	archive.Summary = UTF8Summary(archive.Content, 100)
	return archive, err
}

func (s *Store) FamilyArchiveCreatorID(ctx context.Context, id int64) (int64, error) {
	var creatorID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT created_by FROM archives WHERE id=? AND visibility='family'`, id).Scan(&creatorID); err != nil {
		return 0, err
	}
	return creatorID, nil
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
	var familyArchive int
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM archives WHERE id=? AND visibility='family')`, archiveID).Scan(&familyArchive); err != nil {
		return err
	}
	if familyArchive == 0 {
		return errors.New("共享资料不存在")
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
