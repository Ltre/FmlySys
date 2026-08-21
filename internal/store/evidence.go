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

const EvidenceMaxFileSize int64 = 10 << 20
const EvidenceMaxFiles = 20

var evidenceExts = map[string]bool{"jpg": true, "jpeg": true, "png": true, "gif": true, "webp": true, "bmp": true, "heic": true, "heif": true, "pdf": true, "txt": true, "doc": true, "docx": true, "xls": true, "xlsx": true, "ppt": true, "pptx": true}

func validateEvidenceHeader(h *multipart.FileHeader) error {
	if h == nil || h.Size <= 0 {
		return errors.New("凭证文件为空")
	}
	if h.Size > EvidenceMaxFileSize {
		return fmt.Errorf("文件 %s 超过 10MB", filepath.Base(h.Filename))
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(h.Filename), "."))
	if !evidenceExts[ext] {
		return fmt.Errorf("文件 %s 类型不支持", filepath.Base(h.Filename))
	}
	return nil
}

func ValidateEvidenceFiles(headers []*multipart.FileHeader) error {
	if len(headers) > EvidenceMaxFiles {
		return fmt.Errorf("一次最多上传 %d 个支付/转账凭证", EvidenceMaxFiles)
	}
	for _, h := range headers {
		if err := validateEvidenceHeader(h); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveEvidenceFiles(ctx context.Context, actor int64, entityType string, entityID int64, uploadDir string, headers []*multipart.FileHeader) error {
	if len(headers) == 0 {
		return nil
	}
	if len(headers) > EvidenceMaxFiles {
		return fmt.Errorf("一次最多上传 %d 个支付/转账凭证", EvidenceMaxFiles)
	}
	for _, h := range headers {
		if err := validateEvidenceHeader(h); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var finals []string
	cleanup := func() {
		for _, p := range finals {
			_ = os.Remove(p)
		}
	}
	for _, h := range headers {
		src, err := h.Open()
		if err != nil {
			cleanup()
			return err
		}
		tmp, err := os.CreateTemp(uploadDir, "evidence-*")
		if err != nil {
			src.Close()
			cleanup()
			return err
		}
		tmpName := tmp.Name()
		hash := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(src, EvidenceMaxFileSize+1))
		src.Close()
		closeErr := tmp.Close()
		if copyErr != nil || closeErr != nil || n > EvidenceMaxFileSize {
			_ = os.Remove(tmpName)
			cleanup()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			return fmt.Errorf("文件 %s 超过 10MB", filepath.Base(h.Filename))
		}
		sum := hex.EncodeToString(hash.Sum(nil))
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(h.Filename), "."))
		storage := sum[:20] + "-" + fmt.Sprintf("%d", time.Now().UnixNano()) + "." + ext
		final := filepath.Join(uploadDir, storage)
		if err := os.Rename(tmpName, final); err != nil {
			_ = os.Remove(tmpName)
			cleanup()
			return err
		}
		finals = append(finals, final)
		res, err := tx.ExecContext(ctx, `INSERT INTO record_attachments(entity_type,entity_id,storage_name,original_name,mime_type,size,sha256,uploaded_by,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, entityType, entityID, storage, filepath.Base(h.Filename), h.Header.Get("Content-Type"), n, sum, actor, now())
		if err != nil {
			cleanup()
			return err
		}
		id, _ := res.LastInsertId()
		if err := auditTx(ctx, tx, actor, "create", "record_attachment", id, nil, map[string]any{"entity_type": entityType, "entity_id": entityID, "original_name": filepath.Base(h.Filename), "sha256": sum}); err != nil {
			cleanup()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		return err
	}
	return nil
}

func (s *Store) EvidenceFor(ctx context.Context, entityType string, entityID int64) ([]Evidence, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,entity_type,entity_id,original_name,mime_type,size FROM record_attachments WHERE entity_type=? AND entity_id=? ORDER BY id`, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Evidence
	for rows.Next() {
		var v Evidence
		if err := rows.Scan(&v.ID, &v.EntityType, &v.EntityID, &v.OriginalName, &v.MimeType, &v.Size); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) EvidencePath(ctx context.Context, id int64, uploadDir string) (string, string, error) {
	var storage, original string
	if err := s.DB.QueryRowContext(ctx, `SELECT storage_name,original_name FROM record_attachments WHERE id=?`, id).Scan(&storage, &original); err != nil {
		return "", "", err
	}
	return filepath.Join(uploadDir, storage), original, nil
}
