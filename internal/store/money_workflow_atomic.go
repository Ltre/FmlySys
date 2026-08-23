package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
	"time"
)

const moneyWorkflowDBTimeout = 4 * time.Second

var moneyWorkflowGate = make(chan struct{}, 1)

func acquireMoneyWorkflow(ctx context.Context) (func(), error) {
	timer := time.NewTimer(moneyWorkflowDBTimeout)
	defer timer.Stop()
	select {
	case moneyWorkflowGate <- struct{}{}:
		return func() { <-moneyWorkflowGate }, nil
	case <-ctx.Done():
		return nil, moneyWorkflowError(ctx.Err())
	case <-timer.C:
		return nil, errors.New("数据库写入繁忙超过 4 秒，请稍后重试；本次操作未开始写入")
	}
}

func moneyWorkflowContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, moneyWorkflowDBTimeout)
}

func moneyWorkflowError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errors.New("数据库写入等待超时，请稍后重试；本次操作未完成")
	}
	return err
}

func holderBalanceTx(ctx context.Context, tx *sql.Tx, memberID int64) (int64, error) {
	var base, incoming, outgoing, spent, reimb int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE event_type WHEN 'ASSET_OUT' THEN -amount_cent ELSE amount_cent END),0) FROM asset_events WHERE holder_member_id=? AND status='active'`, memberID).Scan(&base); err != nil {
		return 0, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM holder_transfers WHERE to_member_id=? AND status='active'`, memberID).Scan(&incoming); err != nil {
		return 0, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM holder_transfers WHERE from_member_id=? AND status='active'`, memberID).Scan(&outgoing); err != nil {
		return 0, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(public_paid_amount_cent),0) FROM public_expenses WHERE COALESCE(holder_member_id,handler_member_id)=? AND status='active'`, memberID).Scan(&spent); err != nil {
		return 0, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM reimbursements WHERE payer_holder_member_id=? AND status='active'`, memberID).Scan(&reimb); err != nil {
		return 0, err
	}
	return base + incoming - outgoing - spent - reimb, nil
}

func insertExpenseAutoTx(ctx context.Context, tx *sql.Tx, actor int64, e ExpenseInputV2) (int64, error) {
	if strings.TrimSpace(e.Title) == "" || e.AmountCent <= 0 || e.HandlerMemberID == 0 {
		return 0, errors.New("消费事项、金额和经手人不能为空")
	}
	if err := validatePaymentChannel(e.PaymentChannel); err != nil {
		return 0, err
	}
	bal, err := holderBalanceTx(ctx, tx, e.HandlerMemberID)
	if err != nil {
		return 0, err
	}
	publicPaid := e.AmountCent
	if bal < publicPaid {
		publicPaid = bal
	}
	if publicPaid < 0 {
		publicPaid = 0
	}
	reimbursable := e.AmountCent - publicPaid
	funding := "PUBLIC_HELD_ASSET"
	if reimbursable > 0 {
		funding = "PERSONAL_ADVANCE"
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO public_expenses(title,category,amount_cent,occurred_at,handler_member_id,payer_member_id,funding_type,holder_member_id,payment_channel,merchant,description,matter_id,reimbursable_amount_cent,status,version,created_by,created_at,updated_at,public_paid_amount_cent) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'active',1,?,?,?,?)`, e.Title, e.Category, e.AmountCent, e.OccurredAt, e.HandlerMemberID, e.HandlerMemberID, funding, e.HandlerMemberID, e.PaymentChannel, e.Merchant, e.Description, nullID(e.MatterID), reimbursable, actor, now(), now(), publicPaid)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "expense", id, nil, map[string]any{"title": e.Title, "amount_cent": e.AmountCent, "handler_member_id": e.HandlerMemberID, "payment_channel": e.PaymentChannel, "public_paid_amount_cent": publicPaid, "reimbursable_amount_cent": reimbursable}); err != nil {
		return 0, err
	}
	return id, nil
}

func insertTransferTx(ctx context.Context, tx *sql.Tx, actor, from, to, amount int64, purpose, channel, occurred string, matterID int64) (int64, error) {
	if err := validatePaymentChannel(channel); err != nil {
		return 0, err
	}
	if amount <= 0 || from == 0 || to == 0 || from == to {
		return 0, errors.New("内部转账参数无效")
	}
	bal, err := holderBalanceTx(ctx, tx, from)
	if err != nil {
		return 0, err
	}
	if bal < amount {
		return 0, errors.New("转出人的公共资产虚拟账户余额不足")
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO holder_transfers(from_member_id,to_member_id,amount_cent,purpose,payment_channel,occurred_at,matter_id,status,created_by,created_at) VALUES(?,?,?,?,?,?,?,'active',?,?)`, from, to, amount, purpose, channel, occurred, nullID(matterID), actor, now())
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "holder_transfer", id, nil, map[string]any{"from": from, "to": to, "amount_cent": amount, "payment_channel": channel}); err != nil {
		return 0, err
	}
	return id, nil
}

func insertReimbursementTx(ctx context.Context, tx *sql.Tx, actor, expenseID, holderID, amount int64, channel, occurred, note string) (int64, error) {
	if err := validatePaymentChannel(channel); err != nil {
		return 0, err
	}
	if amount <= 0 {
		return 0, errors.New("报销金额必须大于 0")
	}
	var receiver, reimbursable int64
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT payer_member_id,reimbursable_amount_cent,status FROM public_expenses WHERE id=?`, expenseID).Scan(&receiver, &reimbursable, &status); err != nil {
		return 0, err
	}
	if status != "active" || reimbursable <= 0 {
		return 0, errors.New("该消费不存在有效待报销款")
	}
	var already int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM reimbursements WHERE expense_id=? AND status='active'`, expenseID).Scan(&already); err != nil {
		return 0, err
	}
	if already+amount > reimbursable {
		return 0, errors.New("报销金额超过剩余待报销金额")
	}
	bal, err := holderBalanceTx(ctx, tx, holderID)
	if err != nil {
		return 0, err
	}
	if bal < amount {
		return 0, errors.New("报销付款人的公共资产虚拟账户余额不足")
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO reimbursements(expense_id,payer_holder_member_id,receiver_member_id,amount_cent,payment_channel,occurred_at,note,status,created_by,created_at) VALUES(?,?,?,?,?,?,?,'active',?,?)`, expenseID, holderID, receiver, amount, channel, occurred, note, actor, now())
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "reimbursement", id, nil, map[string]any{"expense_id": expenseID, "holder": holderID, "amount_cent": amount, "payment_channel": channel}); err != nil {
		return 0, err
	}
	return id, nil
}

func insertAssetEventTx(ctx context.Context, tx *sql.Tx, actor, holder int64, typ string, amount int64, description, occurred string, relatedEventID int64) (int64, error) {
	if holder == 0 || amount == 0 {
		return 0, errors.New("持有人和金额不能为空")
	}
	if typ != "INITIAL_ASSET" && typ != "ASSET_IN" && typ != "ASSET_OUT" && typ != "ADJUSTMENT" {
		return 0, errors.New("不支持的资产变动方式")
	}
	if typ != "ADJUSTMENT" && amount < 0 {
		return 0, errors.New("该资产变动金额必须为正数")
	}
	if typ == "INITIAL_ASSET" {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM asset_events WHERE holder_member_id=? AND event_type='INITIAL_ASSET' AND status='active'`, holder).Scan(&count); err != nil {
			return 0, err
		}
		if count > 0 {
			return 0, errors.New("该成员已经登记过初始资产，后续流入请使用“资产新增”")
		}
	}
	delta := amount
	if typ == "ASSET_OUT" {
		delta = -amount
	}
	if delta < 0 {
		bal, err := holderBalanceTx(ctx, tx, holder)
		if err != nil {
			return 0, err
		}
		if bal+delta < 0 {
			return 0, errors.New("该操作会导致持有人公共资产虚拟账户透支")
		}
	}
	if typ == "ASSET_OUT" && relatedEventID != 0 {
		var eventHolder int64
		var eventType string
		if err := tx.QueryRowContext(ctx, `SELECT holder_member_id,event_type FROM asset_events WHERE id=? AND status='active'`, relatedEventID).Scan(&eventHolder, &eventType); err != nil {
			return 0, errors.New("关联的原始流入记录不存在")
		}
		if eventHolder != holder || (eventType != "INITIAL_ASSET" && eventType != "ASSET_IN") {
			return 0, errors.New("资产划出只能关联该持有人的初始资产或资产新增记录")
		}
	} else if typ != "ASSET_OUT" {
		relatedEventID = 0
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO asset_events(event_type,amount_cent,holder_member_id,description,occurred_at,status,created_by,created_at,related_event_id) VALUES(?,?,?,?,?,'active',?,?,?)`, typ, amount, holder, description, occurred, actor, now(), nullID(relatedEventID))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "asset_event", id, nil, map[string]any{"event_type": typ, "amount_cent": amount, "holder_member_id": holder, "related_event_id": relatedEventID, "description": description}); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) atomicMoneyWithEvidence(ctx context.Context, actor int64, entityType, uploadDir string, headers []*multipart.FileHeader, insert func(context.Context, *sql.Tx) (int64, error)) (int64, error) {
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
	id, err := insert(dbCtx, tx)
	if err != nil {
		return 0, moneyWorkflowError(err)
	}
	if err := insertWorkflowEvidenceTx(dbCtx, tx, actor, entityType, id, prepared); err != nil {
		return 0, moneyWorkflowError(err)
	}
	if err := tx.Commit(); err != nil {
		return 0, moneyWorkflowError(err)
	}
	committed = true
	return id, nil
}

func (s *Store) CreateExpenseWorkflowAtomic(ctx context.Context, actor int64, e ExpenseInputV2, uploadDir string, headers []*multipart.FileHeader) (int64, error) {
	return s.atomicMoneyWithEvidence(ctx, actor, "expense", uploadDir, headers, func(dbCtx context.Context, tx *sql.Tx) (int64, error) {
		return insertExpenseAutoTx(dbCtx, tx, actor, e)
	})
}

func (s *Store) CreateTransferWorkflowAtomic(ctx context.Context, actor, from, to, amount int64, purpose, channel, occurred string, matterID int64, uploadDir string, headers []*multipart.FileHeader) (int64, error) {
	return s.atomicMoneyWithEvidence(ctx, actor, "transfer", uploadDir, headers, func(dbCtx context.Context, tx *sql.Tx) (int64, error) {
		return insertTransferTx(dbCtx, tx, actor, from, to, amount, purpose, channel, occurred, matterID)
	})
}

func (s *Store) CreateReimbursementWorkflowAtomic(ctx context.Context, actor, expenseID, holderID, amount int64, channel, occurred, note, uploadDir string, headers []*multipart.FileHeader) (int64, error) {
	return s.atomicMoneyWithEvidence(ctx, actor, "reimbursement", uploadDir, headers, func(dbCtx context.Context, tx *sql.Tx) (int64, error) {
		return insertReimbursementTx(dbCtx, tx, actor, expenseID, holderID, amount, channel, occurred, note)
	})
}

func (s *Store) CreateAssetEventWorkflowAtomic(ctx context.Context, actor, holder int64, typ string, amount int64, description, occurred string, relatedEventID int64) (int64, error) {
	return s.atomicMoneyWithEvidence(ctx, actor, "asset_event", "", nil, func(dbCtx context.Context, tx *sql.Tx) (int64, error) {
		return insertAssetEventTx(dbCtx, tx, actor, holder, typ, amount, description, occurred, relatedEventID)
	})
}

func validateQuickAssetType(typ string) error {
	if typ != "ASSET_IN" && typ != "ASSET_OUT" {
		return fmt.Errorf("快速记录的资产变动只允许资产新增或资产减少")
	}
	return nil
}
