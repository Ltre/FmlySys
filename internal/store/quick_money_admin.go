package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type AdminQuickMoneyNote struct {
	ID                     int64
	Category               string
	CategoryLabel          string
	Summary                string
	Status                 string
	StandardizedEntityType string
	StandardizedEntityID   int64
	CreatedBy              int64
	CreatorName            string
	CreatedAt              string
	StandardizedAt         string
	Evidence               []Evidence
}

func scanAdminQuickMoneyNote(row interface{ Scan(...any) error }) (AdminQuickMoneyNote, error) {
	var n AdminQuickMoneyNote
	var entityID sql.NullInt64
	var entityType, standardizedAt sql.NullString
	if err := row.Scan(&n.ID, &n.Category, &n.Summary, &n.Status, &entityType, &entityID, &n.CreatedBy, &n.CreatorName, &n.CreatedAt, &standardizedAt); err != nil {
		return n, err
	}
	n.CategoryLabel = QuickMoneyCategoryName(n.Category)
	if entityType.Valid {
		n.StandardizedEntityType = entityType.String
	}
	if entityID.Valid {
		n.StandardizedEntityID = entityID.Int64
	}
	if standardizedAt.Valid {
		n.StandardizedAt = standardizedAt.String
	}
	return n, nil
}

func (s *Store) AdminQuickMoneyNotes(ctx context.Context) ([]AdminQuickMoneyNote, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT q.id,q.category,q.summary,q.status,q.standardized_entity_type,q.standardized_entity_id,
       q.created_by,
       CASE WHEN m.is_del=1 THEN m.name || '（已删除）' ELSE m.name END,
       q.created_at,q.standardized_at
FROM quick_money_notes q
JOIN members m ON m.id=q.created_by
ORDER BY CASE q.status WHEN 'draft' THEN 0 ELSE 1 END,q.id DESC
LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminQuickMoneyNote
	for rows.Next() {
		n, err := scanAdminQuickMoneyNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Evidence, _ = s.EvidenceFor(ctx, "quick_money_note", out[i].ID)
		if out[i].Status == "standardized" && len(out[i].Evidence) == 0 && out[i].StandardizedEntityType != "" && out[i].StandardizedEntityID > 0 {
			out[i].Evidence, _ = s.EvidenceFor(ctx, out[i].StandardizedEntityType, out[i].StandardizedEntityID)
		}
	}
	return out, nil
}

func (s *Store) AdminQuickMoneyNoteByID(ctx context.Context, id int64) (AdminQuickMoneyNote, error) {
	var memberStatus string
	var isDel int
	var n AdminQuickMoneyNote
	var entityID sql.NullInt64
	var entityType, standardizedAt sql.NullString
	err := s.DB.QueryRowContext(ctx, `
SELECT q.id,q.category,q.summary,q.status,q.standardized_entity_type,q.standardized_entity_id,
       q.created_by,m.name,q.created_at,q.standardized_at,m.status,m.is_del
FROM quick_money_notes q
JOIN members m ON m.id=q.created_by
WHERE q.id=?`, id).Scan(&n.ID, &n.Category, &n.Summary, &n.Status, &entityType, &entityID, &n.CreatedBy, &n.CreatorName, &n.CreatedAt, &standardizedAt, &memberStatus, &isDel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return n, errors.New("快速记录不存在")
		}
		return n, err
	}
	n.CategoryLabel = QuickMoneyCategoryName(n.Category)
	if entityType.Valid {
		n.StandardizedEntityType = entityType.String
	}
	if entityID.Valid {
		n.StandardizedEntityID = entityID.Int64
	}
	if standardizedAt.Valid {
		n.StandardizedAt = standardizedAt.String
	}
	if n.Status != "draft" {
		return n, errors.New("该快速记录已经完成数据入库")
	}
	if memberStatus != "active" || isDel != 0 {
		return n, errors.New("快速记录的提交成员已经删除，不能继续生成新的资金流水")
	}
	n.Evidence, _ = s.EvidenceFor(ctx, "quick_money_note", id)
	return n, nil
}

// StandardizeQuickMoneyNoteForOwner performs the same semantic conversion as
// the member-facing flow, but keeps the original submitter as the money subject
// while recording the administrative actor in created_by/audit fields.
func (s *Store) StandardizeQuickMoneyNoteForOwner(ctx context.Context, ownerMemberID, auditActor, noteID int64, in QuickMoneyStandardizeInput) (string, int64, error) {
	if ownerMemberID <= 0 || auditActor <= 0 {
		return "", 0, errors.New("快速记录成员或后台审计身份无效")
	}
	dbCtx, cancel := moneyWorkflowContext(ctx)
	defer cancel()
	release, err := acquireMoneyWorkflow(dbCtx)
	if err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	defer release()
	tx, err := s.DB.BeginTx(dbCtx, nil)
	if err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	defer tx.Rollback()

	var category, summary, status string
	if err := tx.QueryRowContext(dbCtx, `SELECT category,summary,status FROM quick_money_notes WHERE id=? AND created_by=?`, noteID, ownerMemberID).Scan(&category, &summary, &status); err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	if status != "draft" {
		return "", 0, errors.New("该快速记录已经完成数据入库")
	}
	var active int
	if err := tx.QueryRowContext(dbCtx, `SELECT EXISTS(SELECT 1 FROM members WHERE id=? AND status='active' AND is_del=0)`, ownerMemberID).Scan(&active); err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	if active == 0 {
		return "", 0, errors.New("快速记录的提交成员已经删除，不能继续生成新的资金流水")
	}

	var entityType string
	var entityID int64
	switch category {
	case QuickMoneyExpense:
		entityType = "expense"
		e := ExpenseInputV2{
			Title:           fallbackQuickText(in.Title, summary),
			Category:        in.ExpenseCategory,
			AmountCent:      in.AmountCent,
			OccurredAt:      in.OccurredAt,
			HandlerMemberID: ownerMemberID,
			PaymentChannel:  in.PaymentChannel,
			Merchant:        in.Merchant,
			Description:     fallbackQuickText(in.Description, summary),
			MatterID:        in.MatterID,
		}
		entityID, err = insertExpenseAutoTx(dbCtx, tx, auditActor, e)
	case QuickMoneyTransfer:
		entityType = "transfer"
		from, to := ownerMemberID, in.CounterpartyID
		if in.Direction == "FROM" {
			from, to = in.CounterpartyID, ownerMemberID
		} else if in.Direction != "TO" {
			err = errors.New("请选择转账方向")
			break
		}
		entityID, err = insertTransferTx(dbCtx, tx, auditActor, from, to, in.AmountCent, fallbackQuickText(in.Purpose, summary), in.PaymentChannel, in.OccurredAt, in.MatterID)
	case QuickMoneyReimbursement:
		entityType = "reimbursement"
		entityID, err = insertReimbursementTx(dbCtx, tx, auditActor, in.ExpenseID, ownerMemberID, in.AmountCent, in.PaymentChannel, in.OccurredAt, fallbackQuickText(in.Note, summary))
	case QuickMoneyAssetEvent:
		entityType = "asset_event"
		if typeErr := validateQuickAssetType(in.EventType); typeErr != nil {
			err = typeErr
			break
		}
		entityID, err = insertAssetEventTx(dbCtx, tx, auditActor, ownerMemberID, in.EventType, in.AmountCent, fallbackQuickText(in.Description, summary), in.OccurredAt, 0)
	default:
		err = fmt.Errorf("不支持的快速记录分类：%s", category)
	}
	if err != nil {
		return "", 0, moneyWorkflowError(err)
	}

	if _, err := tx.ExecContext(dbCtx, `UPDATE record_attachments SET entity_type=?,entity_id=? WHERE entity_type='quick_money_note' AND entity_id=?`, entityType, entityID, noteID); err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	standardizedAt := now()
	res, err := tx.ExecContext(dbCtx, `UPDATE quick_money_notes SET status='standardized',standardized_entity_type=?,standardized_entity_id=?,standardized_at=? WHERE id=? AND created_by=? AND status='draft'`, entityType, entityID, standardizedAt, noteID, ownerMemberID)
	if err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return "", 0, errors.New("快速记录状态已经变化，请刷新后重试")
	}
	if err := auditTx(dbCtx, tx, auditActor, "standardize", "quick_money_note", noteID,
		map[string]any{"status": "draft", "created_by": ownerMemberID},
		map[string]any{"status": "standardized", "entity_type": entityType, "entity_id": entityID, "processed_for_member_id": ownerMemberID}); err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	if err := tx.Commit(); err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	return entityType, entityID, nil
}
