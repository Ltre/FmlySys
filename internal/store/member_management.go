package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

func normalizeMemberInfo(name, relation string) (string, string, error) {
	name = strings.TrimSpace(name)
	relation = strings.TrimSpace(relation)
	if name == "" {
		return "", "", errors.New("成员姓名不能为空")
	}
	if utf8.RuneCountInString(name) > 80 {
		return "", "", errors.New("成员姓名最多 80 个字符")
	}
	if utf8.RuneCountInString(relation) > 160 {
		return "", "", errors.New("关系/备注最多 160 个字符")
	}
	return name, relation, nil
}

func (s *Store) UpdateMemberInfo(ctx context.Context, auditActor, memberID int64, name, relation string) error {
	if memberID <= 0 {
		return errors.New("成员不存在")
	}
	name, relation, err := normalizeMemberInfo(name, relation)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldName, oldRelation, status string
	var isDel int
	if err := tx.QueryRowContext(ctx, `SELECT name,relation,status,is_del FROM members WHERE id=?`, memberID).Scan(&oldName, &oldRelation, &status, &isDel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("成员不存在")
		}
		return err
	}
	if status != "active" || isDel != 0 {
		return errors.New("已删除成员不能修改")
	}
	if oldName == name && oldRelation == relation {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE members SET name=?,relation=? WHERE id=? AND status='active' AND is_del=0`, name, relation, memberID); err != nil {
		return err
	}
	if err := auditTx(ctx, tx, auditActor, "update", "member", memberID,
		map[string]any{"name": oldName, "relation": oldRelation},
		map[string]any{"name": name, "relation": relation}); err != nil {
		return err
	}
	return tx.Commit()
}

func tableExistsTx(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)`, name).Scan(&exists); err != nil {
		return false, err
	}
	return exists != 0, nil
}

// SoftDeleteMember permanently keeps the member row and every historical
// business/audit record. A member can only be marked deleted when the current
// public-asset holder balance is exactly zero.
func (s *Store) SoftDeleteMember(ctx context.Context, auditActor, memberID int64) error {
	if memberID <= 0 {
		return errors.New("成员不存在")
	}
	if memberID == auditActor {
		return errors.New("系统开发身份不能删除")
	}

	dbCtx, cancel := moneyWorkflowContext(ctx)
	defer cancel()
	release, err := acquireMoneyWorkflow(dbCtx)
	if err != nil {
		return moneyWorkflowError(err)
	}
	defer release()

	tx, err := s.DB.BeginTx(dbCtx, nil)
	if err != nil {
		return moneyWorkflowError(err)
	}
	defer tx.Rollback()

	var name, relation, status string
	var isDel int
	if err := tx.QueryRowContext(dbCtx, `SELECT name,relation,status,is_del FROM members WHERE id=?`, memberID).Scan(&name, &relation, &status, &isDel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("成员不存在")
		}
		return moneyWorkflowError(err)
	}
	if status != "active" || isDel != 0 {
		return errors.New("成员已经删除")
	}

	balance, err := holderBalanceTx(dbCtx, tx, memberID)
	if err != nil {
		return moneyWorkflowError(err)
	}
	if balance != 0 {
		return fmt.Errorf("该成员仍持有公共资产（当前余额 %d 分）；只有持有资产为 0 时才能标记删除", balance)
	}

	// Authentication and permissions may be revoked, but business facts and
	// audit_logs are deliberately untouched.
	if _, err := tx.ExecContext(dbCtx, `
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
		return moneyWorkflowError(err)
	}
	if _, err := tx.ExecContext(dbCtx, `UPDATE wechat_identities SET member_id=NULL,updated_at=? WHERE member_id=?`, now(), memberID); err != nil {
		return moneyWorkflowError(err)
	}
	if _, err := tx.ExecContext(dbCtx, `DELETE FROM member_sessions WHERE member_id=?`, memberID); err != nil {
		return moneyWorkflowError(err)
	}
	if _, err := tx.ExecContext(dbCtx, `DELETE FROM member_permissions WHERE member_id=?`, memberID); err != nil {
		return moneyWorkflowError(err)
	}
	if err := deletePasskeyAuthStateTx(dbCtx, tx, memberID); err != nil {
		return moneyWorkflowError(err)
	}

	if exists, err := tableExistsTx(dbCtx, tx, "passkey_login_identities"); err != nil {
		return moneyWorkflowError(err)
	} else if exists {
		if sessionsExist, err := tableExistsTx(dbCtx, tx, "passkey_login_sessions"); err != nil {
			return moneyWorkflowError(err)
		} else if sessionsExist {
			if _, err := tx.ExecContext(dbCtx, `DELETE FROM passkey_login_sessions WHERE identity_id IN (SELECT id FROM passkey_login_identities WHERE member_id=?)`, memberID); err != nil {
				return moneyWorkflowError(err)
			}
		}
		if ceremoniesExist, err := tableExistsTx(dbCtx, tx, "passkey_login_ceremonies"); err != nil {
			return moneyWorkflowError(err)
		} else if ceremoniesExist {
			if _, err := tx.ExecContext(dbCtx, `DELETE FROM passkey_login_ceremonies WHERE identity_id IN (SELECT id FROM passkey_login_identities WHERE member_id=?)`, memberID); err != nil {
				return moneyWorkflowError(err)
			}
		}
		if _, err := tx.ExecContext(dbCtx, `UPDATE passkey_login_identities SET member_id=NULL,updated_at=? WHERE member_id=?`, now(), memberID); err != nil {
			return moneyWorkflowError(err)
		}
	}

	before := map[string]any{"name": name, "relation": relation, "status": status, "is_del": isDel}
	if _, err := tx.ExecContext(dbCtx, `UPDATE members SET is_del=1,status='deleted' WHERE id=?`, memberID); err != nil {
		return moneyWorkflowError(err)
	}
	if err := auditTx(dbCtx, tx, auditActor, "soft_delete", "member", memberID, before,
		map[string]any{"name": name, "relation": relation, "status": "deleted", "is_del": true}); err != nil {
		return moneyWorkflowError(err)
	}
	return moneyWorkflowError(tx.Commit())
}
