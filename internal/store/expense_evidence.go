package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
)

// AddExpenseEvidence stages bytes before the transaction and commits the
// attachment metadata and its audit entries together.
func (s *Store) AddExpenseEvidence(ctx context.Context, actor, expenseID int64, uploadDir string, headers []*multipart.FileHeader) error {
	if expenseID <= 0 {
		return errors.New("消费记录不存在")
	}
	if len(headers) == 0 {
		return errors.New("请选择要添加的支付凭证")
	}
	prepared, err := prepareWorkflowEvidence(uploadDir, headers)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			cleanupWorkflowEvidence(prepared)
		}
	}()

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM public_expenses WHERE id=? AND status='active'`, expenseID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("消费记录不存在")
		}
		return err
	}
	if err := insertWorkflowEvidenceTx(ctx, tx, actor, "expense", expenseID, prepared); err != nil {
		return err
	}
	attachments := make([]map[string]any, 0, len(prepared))
	for _, file := range prepared {
		attachments = append(attachments, map[string]any{"original_name": file.original, "sha256": file.sha256})
	}
	if err := auditTx(ctx, tx, actor, "add_evidence", "expense", expenseID, nil, map[string]any{"attachments": attachments}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// DeleteExpenseEvidence only removes an attachment belonging to this active
// expense. Its metadata and audit entries are committed before file cleanup.
func (s *Store) DeleteExpenseEvidence(ctx context.Context, actor, expenseID, attachmentID int64, uploadDir string) error {
	if expenseID <= 0 || attachmentID <= 0 {
		return errors.New("支付凭证不存在")
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
JOIN public_expenses e ON e.id=a.entity_id AND e.status='active'
WHERE a.id=? AND a.entity_type='expense' AND a.entity_id=?`, attachmentID, expenseID).Scan(&storage, &original, &sum); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("支付凭证不存在")
		}
		return err
	}
	if filepath.Base(storage) != storage {
		return errors.New("支付凭证存储路径无效")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM record_attachments WHERE id=? AND entity_type='expense' AND entity_id=?`, attachmentID, expenseID); err != nil {
		return err
	}
	before := map[string]any{"expense_id": expenseID, "original_name": original, "sha256": sum}
	if err := auditTx(ctx, tx, actor, "delete", "record_attachment", attachmentID, before, nil); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "delete_evidence", "expense", expenseID, before, nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(uploadDir, storage)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("支付凭证记录已删除，但存储文件清理失败: %w", err)
	}
	return nil
}
