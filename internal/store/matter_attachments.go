package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (s *Store) DeleteMatterAttachment(ctx context.Context, actor, matterID, attachmentID int64, uploadDir string) error {
	if matterID <= 0 || attachmentID <= 0 {
		return errors.New("事务附件不存在")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var storage, original, sum string
	if err := tx.QueryRowContext(ctx, `
SELECT a.storage_name,a.original_name,a.sha256
FROM record_attachments a
JOIN matters m ON m.id=a.entity_id
WHERE a.id=? AND a.entity_type='matter' AND a.entity_id=?`, attachmentID, matterID).Scan(&storage, &original, &sum); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("事务附件不存在")
		}
		return err
	}
	if filepath.Base(storage) != storage {
		return errors.New("事务附件存储路径无效")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM record_attachments WHERE id=? AND entity_type='matter' AND entity_id=?`, attachmentID, matterID); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "delete", "record_attachment", attachmentID,
		map[string]any{"matter_id": matterID, "original_name": original, "sha256": sum}, nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(uploadDir, storage)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("事务附件记录已删除，但存储文件清理失败: %w", err)
	}
	return nil
}
