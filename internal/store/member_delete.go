package store

import (
	"context"
	"database/sql"
	"errors"
)

const (
	MemberDeleteSoft = "soft"
	MemberDeleteHard = "hard"
)

// MembersForAccounting returns members that must still participate in historical
// ledger calculations. Soft-deleted members remain here so deleting a member
// never changes the public-asset totals that were created by historical facts.
func (s *Store) MembersForAccounting(ctx context.Context) ([]Member, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT id,
       CASE WHEN is_del=1 THEN name || '（已删除）' ELSE name END,
       relation
FROM members
WHERE status='active' OR is_del=1
ORDER BY id`)
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

// DeleteMemberSmart physically deletes an unused member. If persistent business
// or audit data references the member, the member row is retained with is_del=1
// and status=deleted so historical records and foreign-key relationships remain
// intact. Authentication/session state, including Passkeys, is removed in both cases.
func (s *Store) DeleteMemberSmart(ctx context.Context, auditActor, memberID int64) (string, error) {
	if memberID <= 0 {
		return "", errors.New("成员不存在")
	}
	if memberID == auditActor {
		return "", errors.New("系统开发身份不能删除")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var name, relation, status string
	var isDel int
	err = tx.QueryRowContext(ctx, `SELECT name,relation,status,is_del FROM members WHERE id=?`, memberID).Scan(&name, &relation, &status, &isDel)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("成员不存在")
	}
	if err != nil {
		return "", err
	}
	if isDel != 0 || status == "deleted" {
		return "", errors.New("成员已经删除")
	}

	referenced, err := memberHasPersistentReferences(ctx, tx, memberID)
	if err != nil {
		return "", err
	}

	// A deleted member must not retain a usable login identity. Reset the join
	// request before detaching the WeChat identity so the original openid can
	// later submit a fresh request and be bound to another/new member.
	if _, err := tx.ExecContext(ctx, `
UPDATE join_requests
SET status='draft',
    rejection_reason='原绑定成员已删除，请重新提交加入申请',
    access_token_hash='',
    access_token_expires_at='',
    requested_at='',
    reviewed_at='',
    reviewed_by='',
    updated_at=?
WHERE openid IN (SELECT openid FROM wechat_identities WHERE member_id=?)`, now(), memberID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE wechat_identities SET member_id=NULL,updated_at=? WHERE member_id=?`, now(), memberID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM member_sessions WHERE member_id=?`, memberID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM member_permissions WHERE member_id=?`, memberID); err != nil {
		return "", err
	}
	if err := deletePasskeyAuthStateTx(ctx, tx, memberID); err != nil {
		return "", err
	}

	before := map[string]any{"name": name, "relation": relation, "status": status, "is_del": isDel}
	mode := MemberDeleteHard
	if referenced {
		mode = MemberDeleteSoft
		if _, err := tx.ExecContext(ctx, `UPDATE members SET is_del=1,status='deleted' WHERE id=?`, memberID); err != nil {
			return "", err
		}
		if err := auditTx(ctx, tx, auditActor, "soft_delete", "member", memberID, before, map[string]any{"is_del": true, "status": "deleted"}); err != nil {
			return "", err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `DELETE FROM members WHERE id=?`, memberID); err != nil {
			return "", err
		}
		if err := auditTx(ctx, tx, auditActor, "hard_delete", "member", memberID, before, map[string]any{"deleted": true}); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return mode, nil
}

func memberHasPersistentReferences(ctx context.Context, tx *sql.Tx, memberID int64) (bool, error) {
	checks := []string{
		`SELECT EXISTS(SELECT 1 FROM asset_events WHERE holder_member_id=? OR created_by=? LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM matters WHERE owner_member_id=? OR created_by=? LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM public_expenses WHERE handler_member_id=? OR payer_member_id=? OR holder_member_id=? OR created_by=? LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM holder_transfers WHERE from_member_id=? OR to_member_id=? OR created_by=? LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM reimbursements WHERE payer_holder_member_id=? OR receiver_member_id=? OR created_by=? LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM archives WHERE created_by=? LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM attachments WHERE uploaded_by=? LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM record_attachments WHERE uploaded_by=? LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM audit_logs WHERE actor_member_id=? LIMIT 1)`,
	}
	args := [][]any{
		{memberID, memberID},
		{memberID, memberID},
		{memberID, memberID, memberID, memberID},
		{memberID, memberID, memberID},
		{memberID, memberID, memberID},
		{memberID},
		{memberID},
		{memberID},
		{memberID},
	}
	for i, query := range checks {
		var found int
		if err := tx.QueryRowContext(ctx, query, args[i]...).Scan(&found); err != nil {
			return false, err
		}
		if found != 0 {
			return true, nil
		}
	}
	return false, nil
}
