package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

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
	if memberID.Valid && memberName.Valid && memberName.String != "" {
		v.MemberID = memberID.Int64
		v.MemberName = memberName.String
	}
	return v, nil
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
