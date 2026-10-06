package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const TOTPLoginIdentitySessionTTL = 30 * 24 * time.Hour

type TOTPLoginIdentity struct {
	ID            int64
	Username      string
	UsernameKey   string
	ProfileRemark string
	SecretEnc     string
	LastTOTPStep  int64
	MemberID      int64
	MemberName    string
	CreatedAt     string
}

func (s *Store) CreateTOTPLoginEnrollment(ctx context.Context, tokenHash, secretEnc string, expiresAt time.Time) error {
	if tokenHash == "" || secretEnc == "" {
		return errors.New("2FA 注册状态无效")
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM totp_login_enrollments WHERE expires_at<=?`, now()); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO totp_login_enrollments(token_hash,secret_enc,expires_at,created_at) VALUES(?,?,?,?)`, tokenHash, secretEnc, expiresAt.UTC().Format(time.RFC3339Nano), now())
	return err
}

func (s *Store) TOTPLoginEnrollmentSecret(ctx context.Context, tokenHash string, at time.Time) (string, error) {
	var secretEnc string
	err := s.DB.QueryRowContext(ctx, `SELECT secret_enc FROM totp_login_enrollments WHERE token_hash=? AND expires_at>?`, tokenHash, at.UTC().Format(time.RFC3339Nano)).Scan(&secretEnc)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("2FA 注册二维码已过期，请刷新页面")
	}
	return secretEnc, err
}

func (s *Store) CreateTOTPLoginIdentity(ctx context.Context, tokenHash, username, usernameKey, remark, secretEnc string, usedStep int64, at time.Time) error {
	username = strings.TrimSpace(username)
	usernameKey = strings.TrimSpace(usernameKey)
	remark = strings.TrimSpace(remark)
	if tokenHash == "" || username == "" || usernameKey == "" || secretEnc == "" {
		return errors.New("2FA 注册信息不完整")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var enrollmentSecret string
	if err := tx.QueryRowContext(ctx, `SELECT secret_enc FROM totp_login_enrollments WHERE token_hash=? AND expires_at>?`, tokenHash, at.UTC().Format(time.RFC3339Nano)).Scan(&enrollmentSecret); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("2FA 注册二维码已过期，请重新扫描")
		}
		return err
	}
	if enrollmentSecret != secretEnc {
		return errors.New("2FA 注册状态已变化，请刷新二维码后重试")
	}
	stamp := now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO totp_login_identities(username,username_key,profile_remark,secret_enc,last_totp_step,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, username, usernameKey, remark, secretEnc, usedStep, stamp, stamp); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
			return errors.New("该用户名已注册，请切换到登录选项卡")
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM totp_login_enrollments WHERE token_hash=?`, tokenHash); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) TOTPLoginIdentityByUsername(ctx context.Context, usernameKey string) (TOTPLoginIdentity, error) {
	var v TOTPLoginIdentity
	var memberID sql.NullInt64
	var memberName sql.NullString
	err := s.DB.QueryRowContext(ctx, `
SELECT t.id,t.username,t.username_key,t.profile_remark,t.secret_enc,t.last_totp_step,
       t.member_id,COALESCE(m.name,''),t.created_at
FROM totp_login_identities t
LEFT JOIN members m ON m.id=t.member_id AND m.status='active' AND m.is_del=0
WHERE t.username_key=?`, usernameKey).Scan(&v.ID, &v.Username, &v.UsernameKey, &v.ProfileRemark, &v.SecretEnc, &v.LastTOTPStep, &memberID, &memberName, &v.CreatedAt)
	if err != nil {
		return TOTPLoginIdentity{}, err
	}
	if memberID.Valid {
		v.MemberID = memberID.Int64
	}
	if memberName.Valid && memberName.String != "" {
		v.MemberName = memberName.String
	}
	return v, nil
}

// CreateTOTPLoginIdentitySession records successful authentication of the
// 2FA identity independently from its optional family-member association.
// It also consumes the TOTP step in the same transaction so the code cannot
// be replayed if session creation fails partway through.
func (s *Store) CreateTOTPLoginIdentitySession(ctx context.Context, identityID, step int64) (string, error) {
	if identityID <= 0 || step < 0 {
		return "", errors.New("2FA 登录状态无效")
	}
	raw, tokenHash, err := memberToken()
	if err != nil {
		return "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	stamp := now()
	res, err := tx.ExecContext(ctx, `
UPDATE totp_login_identities
SET last_totp_step=?,updated_at=?
WHERE id=? AND last_totp_step<?`, step, stamp, identityID, step)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", errors.New("该动态验证码已使用或身份状态已变化，请重试")
	}
	verified := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `
INSERT INTO totp_login_identity_sessions(token_hash,identity_id,expires_at,verified_at,created_at,last_seen_at)
VALUES(?,?,?,?,?,?)`, tokenHash, identityID, time.Now().UTC().Add(TOTPLoginIdentitySessionTTL).Format(time.RFC3339Nano), verified, stamp, stamp)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return raw, nil
}

// TOTPLoginIdentityFromSession returns the current association for a
// previously authenticated 2FA identity. Binding changes made by an
// administrator therefore take effect without requiring another OTP.
func (s *Store) TOTPLoginIdentityFromSession(ctx context.Context, raw string) (TOTPLoginIdentity, error) {
	if raw == "" {
		return TOTPLoginIdentity{}, sql.ErrNoRows
	}
	hash := memberTokenHash(raw)
	var v TOTPLoginIdentity
	var memberID sql.NullInt64
	var expires string
	err := s.DB.QueryRowContext(ctx, `
SELECT t.id,t.username,t.username_key,t.profile_remark,
       t.member_id,COALESCE(m.name,''),t.created_at,s.expires_at
FROM totp_login_identity_sessions s
JOIN totp_login_identities t ON t.id=s.identity_id
LEFT JOIN members m ON m.id=t.member_id AND m.status='active' AND m.is_del=0
WHERE s.token_hash=?`, hash).Scan(&v.ID, &v.Username, &v.UsernameKey, &v.ProfileRemark, &memberID, &v.MemberName, &v.CreatedAt, &expires)
	if err != nil {
		return TOTPLoginIdentity{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || time.Now().UTC().After(expiresAt) {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM totp_login_identity_sessions WHERE token_hash=?`, hash)
		return TOTPLoginIdentity{}, sql.ErrNoRows
	}
	if memberID.Valid {
		v.MemberID = memberID.Int64
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE totp_login_identity_sessions SET last_seen_at=? WHERE token_hash=?`, now(), hash)
	return v, nil
}

func (s *Store) DeleteTOTPLoginIdentitySession(ctx context.Context, raw string) {
	if raw != "" {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM totp_login_identity_sessions WHERE token_hash=?`, memberTokenHash(raw))
	}
}

func (s *Store) CreateTOTPLoginMemberSession(ctx context.Context, identityID, memberID, step int64) (string, error) {
	if identityID <= 0 || memberID <= 0 || step < 0 {
		return "", errors.New("2FA 登录状态无效")
	}
	raw, tokenHash, err := memberToken()
	if err != nil {
		return "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
UPDATE totp_login_identities
SET last_totp_step=?,updated_at=?
WHERE id=? AND member_id=? AND last_totp_step<?
  AND EXISTS(SELECT 1 FROM members WHERE id=? AND status='active' AND is_del=0)`,
		step, now(), identityID, memberID, step, memberID)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", errors.New("该动态验证码已使用或身份关联已变更，请重试")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO member_sessions(token_hash,member_id,expires_at,created_at,last_seen_at) VALUES(?,?,?,?,?)`,
		tokenHash, memberID, time.Now().UTC().Add(MemberSessionTTL).Format(time.RFC3339Nano), now(), now())
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return raw, nil
}

func (s *Store) AllTOTPLoginIdentities(ctx context.Context) ([]TOTPLoginIdentity, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT t.id,t.username,t.username_key,t.profile_remark,t.last_totp_step,
       COALESCE(t.member_id,0),COALESCE(m.name,''),t.created_at
FROM totp_login_identities t
LEFT JOIN members m ON m.id=t.member_id AND m.status='active' AND m.is_del=0
ORDER BY t.created_at DESC,t.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TOTPLoginIdentity
	for rows.Next() {
		var v TOTPLoginIdentity
		if err := rows.Scan(&v.ID, &v.Username, &v.UsernameKey, &v.ProfileRemark, &v.LastTOTPStep, &v.MemberID, &v.MemberName, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type MemberTOTPLoginMethod struct {
	Username      string
	ProfileRemark string
	CreatedAt     string
}

type MemberPasskeyLoginMethod struct {
	Phone            string
	IdentityRemark   string
	CredentialRemark string
	RPID             string
	CreatedAt        string
	LastUsedAt       string
}

type MemberWeChatLoginMethod struct {
	Nickname  string
	CreatedAt string
}

func (s *Store) TOTPLoginMethodsForMember(ctx context.Context, memberID int64) ([]MemberTOTPLoginMethod, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT username,profile_remark,created_at
FROM totp_login_identities WHERE member_id=? ORDER BY created_at DESC,id DESC`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemberTOTPLoginMethod
	for rows.Next() {
		var v MemberTOTPLoginMethod
		if err := rows.Scan(&v.Username, &v.ProfileRemark, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) PasskeyLoginMethodsForMember(ctx context.Context, memberID int64) ([]MemberPasskeyLoginMethod, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT p.phone,p.profile_remark,c.remark,c.rp_id,c.created_at,COALESCE(c.last_used_at,'')
FROM passkey_login_credentials c
JOIN passkey_login_identities p ON p.id=c.identity_id
WHERE COALESCE(c.member_id,p.member_id)=?
ORDER BY c.created_at DESC,c.id DESC`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemberPasskeyLoginMethod
	for rows.Next() {
		var v MemberPasskeyLoginMethod
		if err := rows.Scan(&v.Phone, &v.IdentityRemark, &v.CredentialRemark, &v.RPID, &v.CreatedAt, &v.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) WeChatLoginMethodsForMember(ctx context.Context, memberID int64) ([]MemberWeChatLoginMethod, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT nickname,created_at FROM wechat_identities
WHERE member_id=? ORDER BY created_at DESC,openid`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemberWeChatLoginMethod
	for rows.Next() {
		var v MemberWeChatLoginMethod
		if err := rows.Scan(&v.Nickname, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) BindTOTPLoginIdentity(ctx context.Context, actorID, identityID, memberID int64) error {
	if identityID <= 0 || memberID < 0 {
		return errors.New("2FA 身份或成员 ID 无效")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var username, remark string
	var oldMember sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT username,profile_remark,member_id FROM totp_login_identities WHERE id=?`, identityID).Scan(&username, &remark, &oldMember); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("2FA 登录身份不存在")
		}
		return err
	}
	var memberName string
	if memberID > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT name FROM members WHERE id=? AND status='active' AND is_del=0`, memberID).Scan(&memberName); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("请选择有效的家庭成员")
			}
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE totp_login_identities SET member_id=NULLIF(?,0),updated_at=? WHERE id=?`, memberID, now(), identityID); err != nil {
		return err
	}
	var oldMemberID int64
	if oldMember.Valid {
		oldMemberID = oldMember.Int64
	}
	if err := auditTx(ctx, tx, actorID, "bind_member", "totp_login_identity", identityID,
		map[string]any{"username": username, "remark": remark, "member_id": oldMemberID},
		map[string]any{"username": username, "remark": remark, "member_id": memberID, "member_name": memberName}); err != nil {
		return err
	}
	return tx.Commit()
}

func NormalizeTOTPLoginUsername(username string) (string, string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", "", errors.New("请输入用户名")
	}
	if len([]rune(username)) > 40 {
		return "", "", errors.New("用户名最多 40 个字符")
	}
	key := strings.ToLower(username)
	for _, r := range username {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r >= 0x4e00 && r <= 0x9fff) {
			return "", "", fmt.Errorf("用户名只能包含中英文、数字、点、短横线和下划线")
		}
	}
	return username, key, nil
}
