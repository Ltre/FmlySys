package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Store struct{ DB *sql.DB }

func New(db *sql.DB) *Store { return &Store{DB: db} }

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *Store) EnsureDevMember(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM members WHERE name = ? AND status = 'active' ORDER BY id LIMIT 1`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO members(name, relation, status, created_at) VALUES(?, '系统开发身份', 'active', ?)`, name, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Members(ctx context.Context) ([]Member, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, relation FROM members WHERE status = 'active' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var v Member
		if err := rows.Scan(&v.ID, &v.Name, &v.Relation); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) CreateMember(ctx context.Context, actor int64, name, relation string) error {
	if name == "" {
		return errors.New("成员姓名不能为空")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO members(name, relation, status, created_at) VALUES(?, ?, 'active', ?)`, name, relation, now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "member", id, nil, map[string]any{"name": name, "relation": relation}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AddAssetEvent(ctx context.Context, actor, holder int64, typ string, amount int64, description, occurred string) error {
	if holder == 0 || amount == 0 {
		return errors.New("持有人和金额不能为空")
	}
	if typ != "INITIAL_ASSET" && typ != "ASSET_IN" && typ != "ASSET_OUT" && typ != "ADJUSTMENT" {
		return errors.New("不支持的资产事件")
	}
	if typ != "ADJUSTMENT" && amount < 0 {
		return errors.New("该资产事件金额必须为正数")
	}
	delta := amount
	if typ == "ASSET_OUT" {
		delta = -amount
	}
	if delta < 0 {
		bal, err := s.HolderBalance(ctx, holder)
		if err != nil {
			return err
		}
		if bal+delta < 0 {
			return errors.New("该操作会导致持有人公共资产虚拟账户透支")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO asset_events(event_type, amount_cent, holder_member_id, description, occurred_at, status, created_by, created_at) VALUES(?,?,?,?,?,'active',?,?)`, typ, amount, holder, description, occurred, actor, now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "asset_event", id, nil, map[string]any{"event_type": typ, "amount_cent": amount, "holder_member_id": holder}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateExpense(ctx context.Context, actor int64, e ExpenseInput) error {
	if e.AmountCent <= 0 {
		return errors.New("消费金额必须大于 0")
	}
	if e.FundingType != "PUBLIC_HELD_ASSET" && e.FundingType != "PERSONAL_ADVANCE" {
		return errors.New("不支持的资金来源")
	}
	if e.FundingType == "PUBLIC_HELD_ASSET" && e.HolderMemberID == 0 {
		return errors.New("直接使用公共资产时必须指定资产持有人")
	}
	reimb := int64(0)
	var holder any
	if e.FundingType == "PERSONAL_ADVANCE" {
		reimb = e.AmountCent
		holder = nil
	} else {
		holder = e.HolderMemberID
		bal, err := s.HolderBalance(ctx, e.HolderMemberID)
		if err != nil {
			return err
		}
		if bal < e.AmountCent {
			return fmt.Errorf("持有人可用代管金额不足：当前 %d 分", bal)
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO public_expenses(title, category, amount_cent, occurred_at, handler_member_id, payer_member_id, funding_type, holder_member_id, payment_channel, merchant, description, matter_id, reimbursable_amount_cent, status, version, created_by, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'active',1,?,?,?)`, e.Title, e.Category, e.AmountCent, e.OccurredAt, e.HandlerMemberID, e.PayerMemberID, e.FundingType, holder, e.PaymentChannel, e.Merchant, e.Description, nullID(e.MatterID), reimb, actor, now(), now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "expense", id, nil, map[string]any{"title": e.Title, "amount_cent": e.AmountCent, "funding_type": e.FundingType}); err != nil {
		return err
	}
	return tx.Commit()
}

type ExpenseInput struct {
	Title           string
	Category        string
	AmountCent      int64
	OccurredAt      string
	HandlerMemberID int64
	PayerMemberID   int64
	FundingType     string
	HolderMemberID  int64
	PaymentChannel  string
	Merchant        string
	Description     string
	MatterID        int64
}

func (s *Store) CreateTransfer(ctx context.Context, actor, from, to, amount int64, purpose, channel, occurred string, matterID int64) error {
	if amount <= 0 || from == 0 || to == 0 || from == to {
		return errors.New("内部转账参数无效")
	}
	bal, err := s.HolderBalance(ctx, from)
	if err != nil {
		return err
	}
	if bal < amount {
		return errors.New("转出人的公共资产虚拟账户余额不足")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO holder_transfers(from_member_id,to_member_id,amount_cent,purpose,payment_channel,occurred_at,matter_id,status,created_by,created_at) VALUES(?,?,?,?,?,?,?,'active',?,?)`, from, to, amount, purpose, channel, occurred, nullID(matterID), actor, now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "holder_transfer", id, nil, map[string]any{"from": from, "to": to, "amount_cent": amount}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateReimbursement(ctx context.Context, actor, expenseID, holderID, amount int64, channel, occurred, note string) error {
	if amount <= 0 {
		return errors.New("报销金额必须大于 0")
	}
	var payer, reimbursable int64
	var funding, status string
	if err := s.DB.QueryRowContext(ctx, `SELECT payer_member_id, funding_type, reimbursable_amount_cent, status FROM public_expenses WHERE id = ?`, expenseID).Scan(&payer, &funding, &reimbursable, &status); err != nil {
		return err
	}
	if status != "active" || funding != "PERSONAL_ADVANCE" {
		return errors.New("该消费不存在有效待报销款")
	}
	var already int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM reimbursements WHERE expense_id = ? AND status = 'active'`, expenseID).Scan(&already); err != nil {
		return err
	}
	if already+amount > reimbursable {
		return errors.New("报销金额超过剩余待报销金额")
	}
	bal, err := s.HolderBalance(ctx, holderID)
	if err != nil {
		return err
	}
	if bal < amount {
		return errors.New("报销付款人的公共资产虚拟账户余额不足")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO reimbursements(expense_id,payer_holder_member_id,receiver_member_id,amount_cent,payment_channel,occurred_at,note,status,created_by,created_at) VALUES(?,?,?,?,?,?,?,'active',?,?)`, expenseID, holderID, payer, amount, channel, occurred, note, actor, now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	if err := auditTx(ctx, tx, actor, "create", "reimbursement", id, nil, map[string]any{"expense_id": expenseID, "holder": holderID, "amount_cent": amount}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AssetSummary(ctx context.Context) (AssetSummary, error) {
	members, err := s.Members(ctx)
	if err != nil {
		return AssetSummary{}, err
	}
	var out AssetSummary
	for _, m := range members {
		bal, err := s.HolderBalance(ctx, m.ID)
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
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(e.reimbursable_amount_cent - COALESCE((SELECT SUM(r.amount_cent) FROM reimbursements r WHERE r.expense_id=e.id AND r.status='active'),0)),0) FROM public_expenses e WHERE e.status='active' AND e.funding_type='PERSONAL_ADVANCE'`).Scan(&out.PendingCent); err != nil {
		return out, err
	}
	out.DifferenceCent = out.HolderTotalCent - out.PendingCent - out.NetCent
	return out, nil
}

func (s *Store) HolderBalance(ctx context.Context, memberID int64) (int64, error) {
	var base, incoming, outgoing, direct, reimb int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE event_type WHEN 'ASSET_OUT' THEN -amount_cent ELSE amount_cent END),0) FROM asset_events WHERE holder_member_id=? AND status='active'`, memberID).Scan(&base); err != nil {
		return 0, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM holder_transfers WHERE to_member_id=? AND status='active'`, memberID).Scan(&incoming); err != nil {
		return 0, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM holder_transfers WHERE from_member_id=? AND status='active'`, memberID).Scan(&outgoing); err != nil {
		return 0, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM public_expenses WHERE holder_member_id=? AND funding_type='PUBLIC_HELD_ASSET' AND status='active'`, memberID).Scan(&direct); err != nil {
		return 0, err
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM reimbursements WHERE payer_holder_member_id=? AND status='active'`, memberID).Scan(&reimb); err != nil {
		return 0, err
	}
	return base + incoming - outgoing - direct - reimb, nil
}

func (s *Store) Expenses(ctx context.Context) ([]Expense, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.title,e.category,e.amount_cent,e.occurred_at,e.funding_type,p.name,COALESCE(h.name,''),e.reimbursable_amount_cent,COALESCE((SELECT SUM(r.amount_cent) FROM reimbursements r WHERE r.expense_id=e.id AND r.status='active'),0),e.description,e.payment_channel,e.merchant,COALESCE(m.title,'') FROM public_expenses e JOIN members p ON p.id=e.payer_member_id LEFT JOIN members h ON h.id=e.holder_member_id LEFT JOIN matters m ON m.id=e.matter_id WHERE e.status='active' ORDER BY e.occurred_at DESC,e.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

func (s *Store) ExpenseByID(ctx context.Context, id int64) (Expense, error) {
	var v Expense
	var reimbursed int64
	err := s.DB.QueryRowContext(ctx, `SELECT e.id,e.title,e.category,e.amount_cent,e.occurred_at,e.funding_type,p.name,COALESCE(h.name,''),e.reimbursable_amount_cent,COALESCE((SELECT SUM(r.amount_cent) FROM reimbursements r WHERE r.expense_id=e.id AND r.status='active'),0),e.description,e.payment_channel,e.merchant,COALESCE(m.title,'') FROM public_expenses e JOIN members p ON p.id=e.payer_member_id LEFT JOIN members h ON h.id=e.holder_member_id LEFT JOIN matters m ON m.id=e.matter_id WHERE e.id=? AND e.status='active'`, id).Scan(&v.ID, &v.Title, &v.Category, &v.AmountCent, &v.OccurredAt, &v.FundingType, &v.PayerName, &v.HolderName, &v.ReimbursableCent, &reimbursed, &v.Description, &v.PaymentChannel, &v.Merchant, &v.MatterTitle)
	if err != nil {
		return v, err
	}
	v.ReimbursedCent = reimbursed
	v.PendingCent = v.ReimbursableCent - reimbursed
	return v, nil
}

type ExpenseUpdate struct {
	Title, Category, OccurredAt, PaymentChannel, Merchant, Description string
	AmountCent, MatterID                                               int64
}

func (s *Store) UpdateExpense(ctx context.Context, actor, id int64, in ExpenseUpdate) error {
	if in.AmountCent <= 0 || in.Title == "" {
		return errors.New("消费事项和金额不能为空")
	}
	var old ExpenseUpdate
	var funding string
	var holder sql.NullInt64
	var reimbursable int64
	if err := s.DB.QueryRowContext(ctx, `SELECT title,category,amount_cent,occurred_at,payment_channel,merchant,description,COALESCE(matter_id,0),funding_type,holder_member_id,reimbursable_amount_cent FROM public_expenses WHERE id=? AND status='active'`, id).Scan(&old.Title, &old.Category, &old.AmountCent, &old.OccurredAt, &old.PaymentChannel, &old.Merchant, &old.Description, &old.MatterID, &funding, &holder, &reimbursable); err != nil {
		return err
	}
	if funding == "PERSONAL_ADVANCE" {
		var paid int64
		if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cent),0) FROM reimbursements WHERE expense_id=? AND status='active'`, id).Scan(&paid); err != nil {
			return err
		}
		if in.AmountCent < paid {
			return errors.New("新金额不能小于已经完成的报销金额")
		}
	} else if holder.Valid {
		bal, err := s.HolderBalance(ctx, holder.Int64)
		if err != nil {
			return err
		}
		availableBeforeThisExpense := bal + old.AmountCent
		if availableBeforeThisExpense < in.AmountCent {
			return errors.New("修改后会导致持有人公共资产虚拟账户透支")
		}
	}
	newReimb := reimbursable
	if funding == "PERSONAL_ADVANCE" {
		newReimb = in.AmountCent
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE public_expenses SET title=?,category=?,amount_cent=?,occurred_at=?,payment_channel=?,merchant=?,description=?,matter_id=?,reimbursable_amount_cent=?,updated_at=?,version=version+1 WHERE id=?`, in.Title, in.Category, in.AmountCent, in.OccurredAt, in.PaymentChannel, in.Merchant, in.Description, nullID(in.MatterID), newReimb, now(), id); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, actor, "update", "expense", id, old, in); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AssetEvents(ctx context.Context) ([]AssetEvent, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.id,e.event_type,e.amount_cent,m.name,e.description,e.occurred_at FROM asset_events e JOIN members m ON m.id=e.holder_member_id WHERE e.status='active' ORDER BY e.occurred_at DESC,e.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssetEvent
	for rows.Next() {
		var v AssetEvent
		if err := rows.Scan(&v.ID, &v.Type, &v.AmountCent, &v.HolderName, &v.Description, &v.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Transfers(ctx context.Context) ([]Transfer, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT t.id,f.name,to_m.name,t.amount_cent,t.purpose,t.payment_channel,t.occurred_at,COALESCE(m.title,'') FROM holder_transfers t JOIN members f ON f.id=t.from_member_id JOIN members to_m ON to_m.id=t.to_member_id LEFT JOIN matters m ON m.id=t.matter_id WHERE t.status='active' ORDER BY t.occurred_at DESC,t.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Transfer
	for rows.Next() {
		var v Transfer
		if err := rows.Scan(&v.ID, &v.FromName, &v.ToName, &v.AmountCent, &v.Purpose, &v.PaymentChannel, &v.OccurredAt, &v.MatterTitle); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Reimbursements(ctx context.Context) ([]Reimbursement, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id,e.title,h.name,recv.name,r.amount_cent,r.payment_channel,r.occurred_at FROM reimbursements r JOIN public_expenses e ON e.id=r.expense_id JOIN members h ON h.id=r.payer_holder_member_id JOIN members recv ON recv.id=r.receiver_member_id WHERE r.status='active' ORDER BY r.occurred_at DESC,r.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reimbursement
	for rows.Next() {
		var v Reimbursement
		if err := rows.Scan(&v.ID, &v.ExpenseTitle, &v.HolderName, &v.ReceiverName, &v.AmountCent, &v.PaymentChannel, &v.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func (s *Store) audit(ctx context.Context, actor int64, action, entity string, entityID int64, before, after any) error {
	return auditExec(ctx, s.DB, actor, action, entity, entityID, before, after)
}

func auditTx(ctx context.Context, tx *sql.Tx, actor int64, action, entity string, entityID int64, before, after any) error {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_member_id,action,entity_type,entity_id,before_json,after_json,created_at) VALUES(?,?,?,?,?,?,?)`, actor, action, entity, entityID, string(b), string(a), now())
	return err
}

func auditExec(ctx context.Context, ex interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, actor int64, action, entity string, entityID int64, before, after any) error {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	_, err := ex.ExecContext(ctx, `INSERT INTO audit_logs(actor_member_id,action,entity_type,entity_id,before_json,after_json,created_at) VALUES(?,?,?,?,?,?,?)`, actor, action, entity, entityID, string(b), string(a), now())
	return err
}
