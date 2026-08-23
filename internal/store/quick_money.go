package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
)

const (
	QuickMoneyExpense       = "expense"
	QuickMoneyTransfer      = "transfer"
	QuickMoneyReimbursement = "reimbursement"
	QuickMoneyAssetEvent    = "asset_event"
)

type QuickMoneyNote struct {
	ID                        int64
	Category                  string
	CategoryLabel             string
	Summary                   string
	Status                    string
	StandardizedEntityType    string
	StandardizedEntityID      int64
	CreatedAt, StandardizedAt string
	Evidence                  []Evidence
}

type QuickMoneyStandardizeInput struct {
	Title           string
	ExpenseCategory string
	AmountCent      int64
	OccurredAt      string
	PaymentChannel  string
	Merchant        string
	Description     string
	MatterID        int64
	Direction       string
	CounterpartyID  int64
	Purpose         string
	ExpenseID       int64
	Note            string
	EventType       string
}

func QuickMoneyCategoryName(v string) string {
	switch v {
	case QuickMoneyExpense:
		return "公共消费"
	case QuickMoneyTransfer:
		return "内部转账"
	case QuickMoneyReimbursement:
		return "登记报销"
	case QuickMoneyAssetEvent:
		return "资产变动登记"
	default:
		return v
	}
}

func validateQuickMoneyCategory(v string) error {
	switch v {
	case QuickMoneyExpense, QuickMoneyTransfer, QuickMoneyReimbursement, QuickMoneyAssetEvent:
		return nil
	default:
		return errors.New("请选择一个有效的记录分类")
	}
}

func normalizeQuickMoneySummary(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("请填写快速记录摘要")
	}
	if len([]rune(v)) > 1000 {
		return "", errors.New("快速记录摘要最多 1000 个字符")
	}
	return v, nil
}

func (s *Store) CreateQuickMoneyNote(ctx context.Context, actor int64, category, summary, uploadDir string, headers []*multipart.FileHeader) (int64, error) {
	if actor == 0 {
		return 0, errors.New("成员身份无效")
	}
	if err := validateQuickMoneyCategory(category); err != nil {
		return 0, err
	}
	var err error
	summary, err = normalizeQuickMoneySummary(summary)
	if err != nil {
		return 0, err
	}
	prepared, err := prepareWorkflowEvidence(uploadDir, headers)
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			cleanupWorkflowEvidence(prepared)
		}
	}()

	dbCtx, cancel := moneyWorkflowContext(ctx)
	defer cancel()
	release, err := acquireMoneyWorkflow(dbCtx)
	if err != nil {
		return 0, err
	}
	defer release()
	tx, err := s.DB.BeginTx(dbCtx, nil)
	if err != nil {
		return 0, moneyWorkflowError(err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(dbCtx, `INSERT INTO quick_money_notes(category,summary,status,created_by,created_at) VALUES(?,?,'draft',?,?)`, category, summary, actor, now())
	if err != nil {
		return 0, moneyWorkflowError(err)
	}
	id, _ := res.LastInsertId()
	if err := auditTx(dbCtx, tx, actor, "create", "quick_money_note", id, nil, map[string]any{"category": category, "summary": summary}); err != nil {
		return 0, moneyWorkflowError(err)
	}
	if err := insertWorkflowEvidenceTx(dbCtx, tx, actor, "quick_money_note", id, prepared); err != nil {
		return 0, moneyWorkflowError(err)
	}
	if err := tx.Commit(); err != nil {
		return 0, moneyWorkflowError(err)
	}
	committed = true
	return id, nil
}

func scanQuickMoneyNote(row interface{ Scan(...any) error }) (QuickMoneyNote, error) {
	var n QuickMoneyNote
	var entityID sql.NullInt64
	var entityType, standardizedAt sql.NullString
	err := row.Scan(&n.ID, &n.Category, &n.Summary, &n.Status, &entityType, &entityID, &n.CreatedAt, &standardizedAt)
	if err != nil {
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

func (s *Store) QuickMoneyNotes(ctx context.Context, actor int64) ([]QuickMoneyNote, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,category,summary,status,standardized_entity_type,standardized_entity_id,created_at,standardized_at FROM quick_money_notes WHERE created_by=? ORDER BY id DESC LIMIT 100`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QuickMoneyNote
	for rows.Next() {
		n, err := scanQuickMoneyNote(rows)
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

func (s *Store) QuickMoneyNoteByID(ctx context.Context, id, actor int64) (QuickMoneyNote, error) {
	n, err := scanQuickMoneyNote(s.DB.QueryRowContext(ctx, `SELECT id,category,summary,status,standardized_entity_type,standardized_entity_id,created_at,standardized_at FROM quick_money_notes WHERE id=? AND created_by=?`, id, actor))
	if err != nil {
		return n, err
	}
	if n.Status != "draft" {
		return n, errors.New("该快速记录已经完成数据入库")
	}
	n.Evidence, _ = s.EvidenceFor(ctx, "quick_money_note", id)
	return n, nil
}

func fallbackQuickText(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (s *Store) StandardizeQuickMoneyNote(ctx context.Context, actor, noteID int64, in QuickMoneyStandardizeInput) (string, int64, error) {
	dbCtx, cancel := moneyWorkflowContext(ctx)
	defer cancel()
	release, err := acquireMoneyWorkflow(dbCtx)
	if err != nil {
		return "", 0, err
	}
	defer release()
	tx, err := s.DB.BeginTx(dbCtx, nil)
	if err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	defer tx.Rollback()

	var category, summary, status string
	if err := tx.QueryRowContext(dbCtx, `SELECT category,summary,status FROM quick_money_notes WHERE id=? AND created_by=?`, noteID, actor).Scan(&category, &summary, &status); err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	if status != "draft" {
		return "", 0, errors.New("该快速记录已经完成数据入库")
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
			HandlerMemberID: actor,
			PaymentChannel:  in.PaymentChannel,
			Merchant:        in.Merchant,
			Description:     fallbackQuickText(in.Description, summary),
			MatterID:        in.MatterID,
		}
		entityID, err = insertExpenseAutoTx(dbCtx, tx, actor, e)
	case QuickMoneyTransfer:
		entityType = "transfer"
		from, to := actor, in.CounterpartyID
		if in.Direction == "FROM" {
			from, to = in.CounterpartyID, actor
		} else if in.Direction != "TO" {
			err = errors.New("请选择转账方向")
			break
		}
		entityID, err = insertTransferTx(dbCtx, tx, actor, from, to, in.AmountCent, fallbackQuickText(in.Purpose, summary), in.PaymentChannel, in.OccurredAt, in.MatterID)
	case QuickMoneyReimbursement:
		entityType = "reimbursement"
		entityID, err = insertReimbursementTx(dbCtx, tx, actor, in.ExpenseID, actor, in.AmountCent, in.PaymentChannel, in.OccurredAt, fallbackQuickText(in.Note, summary))
	case QuickMoneyAssetEvent:
		entityType = "asset_event"
		if typeErr := validateQuickAssetType(in.EventType); typeErr != nil {
			err = typeErr
			break
		}
		entityID, err = insertAssetEventTx(dbCtx, tx, actor, actor, in.EventType, in.AmountCent, fallbackQuickText(in.Description, summary), in.OccurredAt, 0)
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
	res, err := tx.ExecContext(dbCtx, `UPDATE quick_money_notes SET status='standardized',standardized_entity_type=?,standardized_entity_id=?,standardized_at=? WHERE id=? AND created_by=? AND status='draft'`, entityType, entityID, standardizedAt, noteID, actor)
	if err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return "", 0, errors.New("快速记录状态已经变化，请刷新后重试")
	}
	if err := auditTx(dbCtx, tx, actor, "standardize", "quick_money_note", noteID, map[string]any{"status": "draft"}, map[string]any{"status": "standardized", "entity_type": entityType, "entity_id": entityID}); err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	if err := tx.Commit(); err != nil {
		return "", 0, moneyWorkflowError(err)
	}
	return entityType, entityID, nil
}
