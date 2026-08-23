package store

import (
	"context"
	"database/sql"
	"errors"
)

type MoneyRecordLocator struct {
	Kind           string
	ID             int64
	OccurredAt     string
	TypeLabel      string
	HolderName     string
	FromName       string
	ToName         string
	ExpenseTitle   string
	ReceiverName   string
	Title          string
	AmountCent     int64
	PaymentChannel string
	Description    string
	Purpose        string
}

func (s *Store) MoneyRecordByID(ctx context.Context, kind string, id int64) (MoneyRecordLocator, error) {
	var v MoneyRecordLocator
	v.Kind, v.ID = kind, id
	switch kind {
	case "asset_event":
		var typ string
		err := s.DB.QueryRowContext(ctx, `
SELECT e.event_type,e.amount_cent,m.name,e.description,e.occurred_at
FROM asset_events e
JOIN members m ON m.id=e.holder_member_id
WHERE e.id=? AND e.status='active'`, id).Scan(&typ, &v.AmountCent, &v.HolderName, &v.Description, &v.OccurredAt)
		if err != nil {
			return v, err
		}
		v.TypeLabel = (AssetEvent{Type: typ}).TypeLabel()
	case "expense":
		err := s.DB.QueryRowContext(ctx, `
SELECT e.title,e.amount_cent,e.occurred_at,e.payment_channel
FROM public_expenses e
WHERE e.id=? AND e.status='active'`, id).Scan(&v.Title, &v.AmountCent, &v.OccurredAt, &v.PaymentChannel)
		if err != nil {
			return v, err
		}
	case "transfer":
		err := s.DB.QueryRowContext(ctx, `
SELECT f.name,tom.name,t.amount_cent,t.payment_channel,t.purpose,t.occurred_at
FROM holder_transfers t
JOIN members f ON f.id=t.from_member_id
JOIN members tom ON tom.id=t.to_member_id
WHERE t.id=? AND t.status='active'`, id).Scan(&v.FromName, &v.ToName, &v.AmountCent, &v.PaymentChannel, &v.Purpose, &v.OccurredAt)
		if err != nil {
			return v, err
		}
	case "reimbursement":
		err := s.DB.QueryRowContext(ctx, `
SELECT e.title,h.name,recv.name,r.amount_cent,r.payment_channel,r.occurred_at
FROM reimbursements r
JOIN public_expenses e ON e.id=r.expense_id
JOIN members h ON h.id=r.payer_holder_member_id
JOIN members recv ON recv.id=r.receiver_member_id
WHERE r.id=? AND r.status='active'`, id).Scan(&v.ExpenseTitle, &v.HolderName, &v.ReceiverName, &v.AmountCent, &v.PaymentChannel, &v.OccurredAt)
		if err != nil {
			return v, err
		}
	default:
		return v, errors.New("不支持的资金记录类型")
	}
	return v, nil
}

func (s *Store) PasskeyLoginIdentityExistsByPhone(ctx context.Context, phone string) (string, bool, error) {
	normalized, err := normalizePasskeyPhone(phone)
	if err != nil {
		return "", false, err
	}
	var exists int
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM passkey_login_identities WHERE phone=?)`, normalized).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return normalized, false, nil
		}
		return "", false, err
	}
	return normalized, exists != 0, nil
}
