package store

import (
	"context"
	"database/sql"
	"errors"
)

var allowedPaymentChannels = map[string]bool{"支付宝": true, "微信": true, "银行": true, "现金": true, "其它": true}

func validatePaymentChannel(v string) error {
	if !allowedPaymentChannels[v] {
		return errors.New("支付/转账渠道无效")
	}
	return nil
}

func (s *Store) MemberByID(ctx context.Context, id int64) (Member, error) {
	var m Member
	err := s.DB.QueryRowContext(ctx, `SELECT id,name,relation FROM members WHERE id=? AND status='active'`, id).Scan(&m.ID, &m.Name, &m.Relation)
	return m, err
}

func (s *Store) AddAssetEventDetailed(ctx context.Context, actor, holder int64, typ string, amount int64, description, occurred string, relatedEventID int64) (int64, error) {
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
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM asset_events WHERE holder_member_id=? AND event_type='INITIAL_ASSET' AND status='active'`, holder).Scan(&count); err != nil {
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
		bal, err := s.HolderBalanceV2(ctx, holder)
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
		if err := s.DB.QueryRowContext(ctx, `SELECT holder_member_id,event_type FROM asset_events WHERE id=? AND status='active'`, relatedEventID).Scan(&eventHolder, &eventType); err != nil {
			return 0, errors.New("关联的原始流入记录不存在")
		}
		if eventHolder != holder || (eventType != "INITIAL_ASSET" && eventType != "ASSET_IN") {
			return 0, errors.New("资产划出只能关联该持有人的初始资产或资产新增记录")
		}
	} else if typ != "ASSET_OUT" {
		relatedEventID = 0
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO asset_events(event_type,amount_cent,holder_member_id,description,occurred_at,status,created_by,created_at,related_event_id) VALUES(?,?,?,?,?,'active',?,?,?)`, typ, amount, holder, description, occurred, actor, now(), nullID(relatedEventID))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "asset_event", id, nil, map[string]any{"event_type": typ, "amount_cent": amount, "holder_member_id": holder, "related_event_id": relatedEventID, "description": description}); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) AddSelfAssetChange(ctx context.Context, actor int64, typ string, amount int64, description, occurred string) (int64, error) {
	if typ != "ASSET_IN" && typ != "ASSET_OUT" {
		return 0, errors.New("前台只允许登记资产新增或资产减少")
	}
	return s.AddAssetEventDetailed(ctx, actor, actor, typ, amount, description, occurred, 0)
}

func (s *Store) AssetInflowOptions(ctx context.Context) ([]AssetInflowOption, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.holder_member_id,m.name,e.event_type,e.amount_cent,e.description,e.occurred_at FROM asset_events e JOIN members m ON m.id=e.holder_member_id WHERE e.status='active' AND e.event_type IN ('INITIAL_ASSET','ASSET_IN') ORDER BY e.occurred_at DESC,e.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssetInflowOption
	for rows.Next() {
		var v AssetInflowOption
		if err := rows.Scan(&v.ID, &v.HolderID, &v.HolderName, &v.Type, &v.AmountCent, &v.Description, &v.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) AssetEventsDetailed(ctx context.Context) ([]AssetEvent, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.event_type,e.amount_cent,m.name,e.description,e.occurred_at,COALESCE(e.related_event_id,0),COALESCE(src.description,'') FROM asset_events e JOIN members m ON m.id=e.holder_member_id LEFT JOIN asset_events src ON src.id=e.related_event_id WHERE e.status='active' ORDER BY e.occurred_at DESC,e.id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssetEvent
	for rows.Next() {
		var v AssetEvent
		if err := rows.Scan(&v.ID, &v.Type, &v.AmountCent, &v.HolderName, &v.Description, &v.OccurredAt, &v.RelatedEventID, &v.RelatedLabel); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type ExpenseInputV2 struct {
	Title, Category, OccurredAt, PaymentChannel, Merchant, Description string
	AmountCent, HandlerMemberID, MatterID                              int64
}

func (s *Store) CreateExpenseAuto(ctx context.Context, actor int64, e ExpenseInputV2) (int64, error) {
	if e.Title == "" || e.AmountCent <= 0 || e.HandlerMemberID == 0 {
		return 0, errors.New("消费事项、金额和经手人不能为空")
	}
	if err := validatePaymentChannel(e.PaymentChannel); err != nil {
		return 0, err
	}
	bal, err := s.HolderBalanceV2(ctx, e.HandlerMemberID)
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
	reimb := e.AmountCent - publicPaid
	funding := "PUBLIC_HELD_ASSET"
	if reimb > 0 {
		funding = "PERSONAL_ADVANCE"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO public_expenses(title,category,amount_cent,occurred_at,handler_member_id,payer_member_id,funding_type,holder_member_id,payment_channel,merchant,description,matter_id,reimbursable_amount_cent,status,version,created_by,created_at,updated_at,public_paid_amount_cent) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'active',1,?,?,?,?)`, e.Title, e.Category, e.AmountCent, e.OccurredAt, e.HandlerMemberID, e.HandlerMemberID, funding, e.HandlerMemberID, e.PaymentChannel, e.Merchant, e.Description, nullID(e.MatterID), reimb, actor, now(), now(), publicPaid)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "expense", id, nil, map[string]any{"title": e.Title, "amount_cent": e.AmountCent, "handler_member_id": e.HandlerMemberID, "payment_channel": e.PaymentChannel, "public_paid_amount_cent": publicPaid, "reimbursable_amount_cent": reimb}); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) CreateTransferV2(ctx context.Context, actor, from, to, amount int64, purpose, channel, occurred string, matterID int64) (int64, error) {
	if err := validatePaymentChannel(channel); err != nil {
		return 0, err
	}
	if amount <= 0 || from == 0 || to == 0 || from == to {
		return 0, errors.New("内部转账参数无效")
	}
	bal, err := s.HolderBalanceV2(ctx, from)
	if err != nil {
		return 0, err
	}
	if bal < amount {
		return 0, errors.New("转出人的公共资产虚拟账户余额不足")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO holder_transfers(from_member_id,to_member_id,amount_cent,purpose,payment_channel,occurred_at,matter_id,status,created_by,created_at) VALUES(?,?,?,?,?,?,?,'active',?,?)`, from, to, amount, purpose, channel, occurred, nullID(matterID), actor, now())
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "holder_transfer", id, nil, map[string]any{"from": from, "to": to, "amount_cent": amount, "payment_channel": channel}); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) CreateReimbursementV2(ctx context.Context, actor, expenseID, holderID, amount int64, channel, occurred, note string) (int64, error) {
	if err := validatePaymentChannel(channel); err != nil {
		return 0, err
	}
	if amount <= 0 {
		return 0, errors.New("报销金额必须大于 0")
	}
	var receiver, reimbursable int64
	var status string
	if err := s.DB.QueryRowContext(ctx, `SELECT payer_member_id,reimbursable_amount_cent,status FROM public_expenses WHERE id=?`, expenseID).Scan(&receiver, &reimbursable, &status); err != nil {
		return 0, err
	}
	if status != "active" || reimbursable <= 0 {
		return 0, errors.New("该消费不存在有效待报销款")
	}
	var already int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM reimbursements WHERE expense_id=? AND status='active'`, expenseID).Scan(&already); err != nil {
		return 0, err
	}
	if already+amount > reimbursable {
		return 0, errors.New("报销金额超过剩余待报销金额")
	}
	bal, err := s.HolderBalanceV2(ctx, holderID)
	if err != nil {
		return 0, err
	}
	if bal < amount {
		return 0, errors.New("报销付款人的公共资产虚拟账户余额不足")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO reimbursements(expense_id,payer_holder_member_id,receiver_member_id,amount_cent,payment_channel,occurred_at,note,status,created_by,created_at) VALUES(?,?,?,?,?,?,?,'active',?,?)`, expenseID, holderID, receiver, amount, channel, occurred, note, actor, now())
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "reimbursement", id, nil, map[string]any{"expense_id": expenseID, "holder": holderID, "amount_cent": amount, "payment_channel": channel}); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) ExpenseAuditLogs(ctx context.Context, expenseID int64) ([]AuditLog, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT a.id,COALESCE(m.name,'系统'),a.action,COALESCE(a.before_json,''),COALESCE(a.after_json,''),a.reason,a.created_at FROM audit_logs a LEFT JOIN members m ON m.id=a.actor_member_id WHERE a.entity_type='expense' AND a.entity_id=? ORDER BY a.id DESC`, expenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditLog
	for rows.Next() {
		var v AuditLog
		if err := rows.Scan(&v.ID, &v.ActorName, &v.Action, &v.BeforeJSON, &v.AfterJSON, &v.Reason, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ReimbursementsForExpense(ctx context.Context, expenseID int64) ([]Reimbursement, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id,r.expense_id,e.title,h.name,recv.name,r.amount_cent,r.payment_channel,r.occurred_at,r.note FROM reimbursements r JOIN public_expenses e ON e.id=r.expense_id JOIN members h ON h.id=r.payer_holder_member_id JOIN members recv ON recv.id=r.receiver_member_id WHERE r.expense_id=? AND r.status='active' ORDER BY r.occurred_at DESC,r.id DESC`, expenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reimbursement
	for rows.Next() {
		var v Reimbursement
		if err := rows.Scan(&v.ID, &v.ExpenseID, &v.ExpenseTitle, &v.HolderName, &v.ReceiverName, &v.AmountCent, &v.PaymentChannel, &v.OccurredAt, &v.Note); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) HolderBalanceV2(ctx context.Context, memberID int64) (int64, error) {
	var base, incoming, outgoing, spent, reimb int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE event_type WHEN 'ASSET_OUT' THEN -amount_cent ELSE amount_cent END),0) FROM asset_events WHERE holder_member_id=? AND status='active'`, memberID).Scan(&base); err != nil {
		return 0, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM holder_transfers WHERE to_member_id=? AND status='active'`, memberID).Scan(&incoming); err != nil {
		return 0, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM holder_transfers WHERE from_member_id=? AND status='active'`, memberID).Scan(&outgoing); err != nil {
		return 0, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(public_paid_amount_cent),0) FROM public_expenses WHERE COALESCE(holder_member_id,handler_member_id)=? AND status='active'`, memberID).Scan(&spent); err != nil {
		return 0, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM reimbursements WHERE payer_holder_member_id=? AND status='active'`, memberID).Scan(&reimb); err != nil {
		return 0, err
	}
	return base + incoming - outgoing - spent - reimb, nil
}

func (s *Store) AssetSummaryV2(ctx context.Context) (AssetSummary, error) {
	members, err := s.MembersForAccounting(ctx)
	if err != nil {
		return AssetSummary{}, err
	}
	var out AssetSummary
	for _, m := range members {
		bal, err := s.HolderBalanceV2(ctx, m.ID)
		if err != nil {
			return out, err
		}
		if bal != 0 {
			out.Holders = append(out.Holders, HolderBalance{MemberID: m.ID, Name: m.Name, Cent: bal})
		}
		out.HolderTotalCent += bal
	}
	var assetBase, expenses int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE event_type WHEN 'ASSET_OUT' THEN -amount_cent ELSE amount_cent END),0) FROM asset_events WHERE status='active'`).Scan(&assetBase); err != nil {
		return out, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM public_expenses WHERE status='active'`).Scan(&expenses); err != nil {
		return out, err
	}
	out.NetCent = assetBase - expenses
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(e.reimbursable_amount_cent - COALESCE((SELECT SUM(r.amount_cent) FROM reimbursements r WHERE r.expense_id=e.id AND r.status='active'),0)),0) FROM public_expenses e WHERE e.status='active' AND e.reimbursable_amount_cent>0`).Scan(&out.PendingCent); err != nil {
		return out, err
	}
	out.DifferenceCent = out.HolderTotalCent - out.PendingCent - out.NetCent
	return out, nil
}

func scanExpenseRows(rows *sql.Rows) ([]Expense, error) {
	var out []Expense
	for rows.Next() {
		var v Expense
		if err := rows.Scan(&v.ID, &v.Title, &v.Category, &v.AmountCent, &v.OccurredAt, &v.FundingType, &v.PayerName, &v.HolderName, &v.ReimbursableCent, &v.ReimbursedCent, &v.Description, &v.PaymentChannel, &v.Merchant, &v.MatterTitle); err != nil {
			return nil, err
		}
		v.PendingCent = v.ReimbursableCent - v.ReimbursedCent
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ExpensesV2(ctx context.Context) ([]Expense, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.title,e.category,e.amount_cent,e.occurred_at,e.funding_type,p.name,COALESCE(h.name,''),e.reimbursable_amount_cent,COALESCE((SELECT SUM(r.amount_cent) FROM reimbursements r WHERE r.expense_id=e.id AND r.status='active'),0),e.description,e.payment_channel,e.merchant,COALESCE(m.title,'') FROM public_expenses e JOIN members p ON p.id=e.handler_member_id LEFT JOIN members h ON h.id=e.holder_member_id LEFT JOIN matters m ON m.id=e.matter_id WHERE e.status='active' ORDER BY e.occurred_at DESC,e.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExpenseRows(rows)
}

func (s *Store) ExpenseByIDV2(ctx context.Context, id int64) (Expense, error) {
	var v Expense
	var reimbursed int64
	err := s.DB.QueryRowContext(ctx, `SELECT e.id,e.title,e.category,e.amount_cent,e.occurred_at,e.funding_type,p.name,COALESCE(h.name,''),e.reimbursable_amount_cent,COALESCE((SELECT SUM(r.amount_cent) FROM reimbursements r WHERE r.expense_id=e.id AND r.status='active'),0),e.description,e.payment_channel,e.merchant,COALESCE(m.title,'') FROM public_expenses e JOIN members p ON p.id=e.handler_member_id LEFT JOIN members h ON h.id=e.holder_member_id LEFT JOIN matters m ON m.id=e.matter_id WHERE e.id=? AND e.status='active'`, id).Scan(&v.ID, &v.Title, &v.Category, &v.AmountCent, &v.OccurredAt, &v.FundingType, &v.PayerName, &v.HolderName, &v.ReimbursableCent, &reimbursed, &v.Description, &v.PaymentChannel, &v.Merchant, &v.MatterTitle)
	if err != nil {
		return v, err
	}
	v.ReimbursedCent = reimbursed
	v.PendingCent = v.ReimbursableCent - reimbursed
	v.Evidence, err = s.EvidenceFor(ctx, "expense", id)
	return v, err
}

func (s *Store) UpdateExpenseV2(ctx context.Context, actor, id int64, in ExpenseUpdate) error {
	if in.AmountCent <= 0 || in.Title == "" {
		return errors.New("消费事项和金额不能为空")
	}
	if err := validatePaymentChannel(in.PaymentChannel); err != nil {
		return err
	}
	var old ExpenseUpdate
	var handler, oldPublicPaid int64
	if err := s.DB.QueryRowContext(ctx, `SELECT title,category,amount_cent,occurred_at,payment_channel,merchant,description,COALESCE(matter_id,0),handler_member_id,public_paid_amount_cent FROM public_expenses WHERE id=? AND status='active'`, id).Scan(&old.Title, &old.Category, &old.AmountCent, &old.OccurredAt, &old.PaymentChannel, &old.Merchant, &old.Description, &old.MatterID, &handler, &oldPublicPaid); err != nil {
		return err
	}
	var paid int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM reimbursements WHERE expense_id=? AND status='active'`, id).Scan(&paid); err != nil {
		return err
	}
	if in.AmountCent < paid {
		return errors.New("新金额不能小于已经完成的报销金额")
	}
	bal, err := s.HolderBalanceV2(ctx, handler)
	if err != nil {
		return err
	}
	available := bal + oldPublicPaid
	if available < 0 {
		available = 0
	}
	publicPaid := in.AmountCent
	if available < publicPaid {
		publicPaid = available
	}
	reimb := in.AmountCent - publicPaid
	if reimb < paid {
		reimb = paid
		publicPaid = in.AmountCent - reimb
	}
	funding := "PUBLIC_HELD_ASSET"
	if reimb > 0 {
		funding = "PERSONAL_ADVANCE"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE public_expenses SET title=?,category=?,amount_cent=?,occurred_at=?,payment_channel=?,merchant=?,description=?,matter_id=?,reimbursable_amount_cent=?,public_paid_amount_cent=?,funding_type=?,holder_member_id=?,payer_member_id=handler_member_id,updated_at=?,version=version+1 WHERE id=?`, in.Title, in.Category, in.AmountCent, in.OccurredAt, in.PaymentChannel, in.Merchant, in.Description, nullID(in.MatterID), reimb, publicPaid, funding, handler, now(), id); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "update", "expense", id, map[string]any{"title": old.Title, "category": old.Category, "amount_cent": old.AmountCent, "occurred_at": old.OccurredAt, "payment_channel": old.PaymentChannel, "merchant": old.Merchant, "description": old.Description, "matter_id": old.MatterID, "public_paid_amount_cent": oldPublicPaid}, map[string]any{"title": in.Title, "category": in.Category, "amount_cent": in.AmountCent, "occurred_at": in.OccurredAt, "payment_channel": in.PaymentChannel, "merchant": in.Merchant, "description": in.Description, "matter_id": in.MatterID, "public_paid_amount_cent": publicPaid, "reimbursable_amount_cent": reimb}); err != nil {
		return err
	}
	return tx.Commit()
}
