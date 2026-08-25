package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type PasskeyCredentialBindingView struct {
	ID                  int64
	RPID                string
	Remark              string
	CreatedAt           string
	LastUsedAt          string
	OverrideMemberID    int64
	OverrideMemberName  string
	EffectiveMemberID   int64
	EffectiveMemberName string
}

type PasskeyIdentityBindingView struct {
	ID                int64
	Phone             string
	ProfileRemark     string
	DefaultMemberID   int64
	DefaultMemberName string
	CreatedAt         string
	UpdatedAt         string
	Credentials       []PasskeyCredentialBindingView
}

func (s *Store) AllPasskeyIdentityBindings(ctx context.Context) ([]PasskeyIdentityBindingView, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT p.id,p.phone,p.profile_remark,COALESCE(p.member_id,0),COALESCE(dm.name,''),p.created_at,p.updated_at
FROM passkey_login_identities p
LEFT JOIN members dm ON dm.id=p.member_id
ORDER BY p.created_at DESC,p.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PasskeyIdentityBindingView
	for rows.Next() {
		var v PasskeyIdentityBindingView
		if err := rows.Scan(&v.ID, &v.Phone, &v.ProfileRemark, &v.DefaultMemberID, &v.DefaultMemberName, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		creds, err := s.PasskeyCredentialBindingViews(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Credentials = creds
	}
	return out, nil
}

func (s *Store) PasskeyCredentialBindingViews(ctx context.Context, identityID int64) ([]PasskeyCredentialBindingView, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT c.id,c.rp_id,c.remark,c.created_at,COALESCE(c.last_used_at,''),
       COALESCE(c.member_id,0),COALESCE(om.name,''),
       COALESCE(c.member_id,p.member_id,0),COALESCE(em.name,'')
FROM passkey_login_credentials c
JOIN passkey_login_identities p ON p.id=c.identity_id
LEFT JOIN members om ON om.id=c.member_id
LEFT JOIN members em ON em.id=COALESCE(c.member_id,p.member_id)
WHERE c.identity_id=?
ORDER BY c.created_at DESC,c.id DESC`, identityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PasskeyCredentialBindingView
	for rows.Next() {
		var v PasskeyCredentialBindingView
		if err := rows.Scan(
			&v.ID,
			&v.RPID,
			&v.Remark,
			&v.CreatedAt,
			&v.LastUsedAt,
			&v.OverrideMemberID,
			&v.OverrideMemberName,
			&v.EffectiveMemberID,
			&v.EffectiveMemberName,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) validateActivePasskeyMember(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, memberID int64) error {
	if memberID <= 0 {
		return errors.New("成员 ID 无效")
	}
	var exists int
	if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM members WHERE id=? AND status='active' AND is_del=0)`, memberID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return errors.New("选择的家族成员不存在或已停用")
	}
	return nil
}

// BindPasskeyCredentialMember changes only one authenticator credential. A
// memberID of 0 means "inherit the login identity's default member".
func (s *Store) BindPasskeyCredentialMember(ctx context.Context, actor, credentialID, memberID int64) error {
	if credentialID <= 0 || memberID < 0 {
		return errors.New("Passkey 凭据或成员 ID 无效")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var identityID int64
	var old sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT identity_id,member_id FROM passkey_login_credentials WHERE id=?`, credentialID).Scan(&identityID, &old); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("Passkey 凭据不存在")
		}
		return err
	}
	var value any
	if memberID > 0 {
		if err := s.validateActivePasskeyMember(ctx, tx, memberID); err != nil {
			return err
		}
		value = memberID
	}
	if _, err := tx.ExecContext(ctx, `UPDATE passkey_login_credentials SET member_id=?,updated_at=? WHERE id=?`, value, now(), credentialID); err != nil {
		return err
	}
	beforeMember := any(nil)
	if old.Valid {
		beforeMember = old.Int64
	}
	afterMember := any(nil)
	if memberID > 0 {
		afterMember = memberID
	}
	if err := auditTx(ctx, tx, actor, "update", "passkey_credential_binding", credentialID,
		map[string]any{"identity_id": identityID, "member_id": beforeMember},
		map[string]any{"identity_id": identityID, "member_id": afterMember}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) BindPasskeyIdentityDefaultAudited(ctx context.Context, actor, identityID, memberID int64) error {
	if identityID <= 0 || memberID < 0 {
		return errors.New("Passkey 登录身份或成员 ID 无效")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT member_id FROM passkey_login_identities WHERE id=?`, identityID).Scan(&old); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("Passkey 登录身份不存在")
		}
		return err
	}
	var value any
	if memberID > 0 {
		if err := s.validateActivePasskeyMember(ctx, tx, memberID); err != nil {
			return err
		}
		value = memberID
	}
	if _, err := tx.ExecContext(ctx, `UPDATE passkey_login_identities SET member_id=?,updated_at=? WHERE id=?`, value, now(), identityID); err != nil {
		return err
	}
	beforeMember := any(nil)
	if old.Valid {
		beforeMember = old.Int64
	}
	afterMember := any(nil)
	if memberID > 0 {
		afterMember = memberID
	}
	if err := auditTx(ctx, tx, actor, "update", "passkey_identity_binding", identityID,
		map[string]any{"member_id": beforeMember}, map[string]any{"member_id": afterMember}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ResolvePasskeyCredentialMember(ctx context.Context, identityID int64, rpID string, credentialID []byte) (int64, error) {
	var memberID int64
	err := s.DB.QueryRowContext(ctx, `
SELECT COALESCE(c.member_id,p.member_id,0)
FROM passkey_login_credentials c
JOIN passkey_login_identities p ON p.id=c.identity_id
WHERE c.identity_id=? AND c.rp_id=? AND c.credential_id=?`, identityID, rpID, credentialID).Scan(&memberID)
	if err != nil {
		return 0, err
	}
	if memberID == 0 {
		return 0, nil
	}
	if err := s.validateActivePasskeyMember(ctx, s.DB, memberID); err != nil {
		return 0, err
	}
	return memberID, nil
}

func (s *Store) CreatePasskeyLoginIdentitySessionForMember(ctx context.Context, identityID, memberID int64) (string, error) {
	raw, hash, err := memberToken()
	if err != nil {
		return "", err
	}
	verified := time.Now().UTC().Format(time.RFC3339Nano)
	var memberValue any
	if memberID > 0 {
		memberValue = memberID
	}
	_, err = s.DB.ExecContext(ctx, `
INSERT INTO passkey_login_sessions(token_hash,identity_id,member_id,expires_at,verified_at,created_at,last_seen_at)
VALUES(?,?,?,?,?,?,?)`, hash, identityID, memberValue, time.Now().UTC().Add(PasskeyLoginIdentitySessionTTL).Format(time.RFC3339Nano), verified, now(), now())
	return raw, err
}

func (s *Store) PasskeyLoginSessionEffectiveMember(ctx context.Context, raw string) (int64, error) {
	if raw == "" {
		return 0, sql.ErrNoRows
	}
	var member sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT member_id FROM passkey_login_sessions WHERE token_hash=?`, memberTokenHash(raw)).Scan(&member); err != nil {
		return 0, err
	}
	if !member.Valid {
		return 0, nil
	}
	return member.Int64, nil
}
