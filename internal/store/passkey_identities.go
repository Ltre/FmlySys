package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const PasskeyLoginIdentitySessionTTL = 30 * 24 * time.Hour
const PasskeyLoginIdentityFreshTTL = 10 * time.Minute

type PasskeyLoginUser struct {
	IdentityID  int64
	Phone       string
	DisplayName string
	MemberID    int64
	MemberName  string
	UserHandle  []byte
	Credentials []webauthn.Credential
}

func (u PasskeyLoginUser) WebAuthnID() []byte                         { return u.UserHandle }
func (u PasskeyLoginUser) WebAuthnName() string                       { return u.Phone }
func (u PasskeyLoginUser) WebAuthnDisplayName() string                { return u.DisplayName }
func (u PasskeyLoginUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

type PasskeyLoginCredentialView struct {
	ID         int64
	RPID       string
	Remark     string
	CreatedAt  string
	LastUsedAt string
}

type PasskeyLoginIdentityView struct {
	ID            int64
	Phone         string
	ProfileRemark string
	MemberID      int64
	MemberName    string
	CreatedAt     string
	UpdatedAt     string
	Credentials   []PasskeyLoginCredentialView
}

type PasskeyLoginCeremony struct {
	Kind       string
	IdentityID int64
	Phone      string
	Remark     string
	Session    webauthn.SessionData
}

func normalizePasskeyPhone(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("请输入手机号")
	}
	var b strings.Builder
	for i, r := range v {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')':
			continue
		default:
			return "", errors.New("手机号只能包含数字、+、空格、短横线和括号")
		}
	}
	phone := b.String()
	if strings.HasPrefix(phone, "0086") {
		phone = "+86" + strings.TrimPrefix(phone, "0086")
	}
	if len(phone) == 11 && phone[0] == '1' {
		phone = "+86" + phone
	}
	digits := strings.TrimPrefix(phone, "+")
	if len(digits) < 6 || len(digits) > 20 {
		return "", errors.New("手机号长度无效")
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return "", errors.New("手机号格式无效")
		}
	}
	return phone, nil
}

func (s *Store) NewPasskeyLoginCandidate(ctx context.Context, phone, remark string) (PasskeyLoginUser, error) {
	phone, err := normalizePasskeyPhone(phone)
	if err != nil {
		return PasskeyLoginUser{}, err
	}
	remark, err = normalizePasskeyRemark(remark)
	if err != nil {
		return PasskeyLoginUser{}, err
	}
	var exists int
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM passkey_login_identities WHERE phone=?)`, phone).Scan(&exists); err != nil {
		return PasskeyLoginUser{}, err
	}
	if exists != 0 {
		return PasskeyLoginUser{}, errors.New("该手机号已经创建过 Passkey 登录身份，请使用“找回已有身份”")
	}
	handle := make([]byte, 32)
	if _, err := rand.Read(handle); err != nil {
		return PasskeyLoginUser{}, err
	}
	return PasskeyLoginUser{Phone: phone, DisplayName: remark, UserHandle: handle}, nil
}

func (s *Store) PasskeyLoginUserByPhone(ctx context.Context, phone, rpID string) (PasskeyLoginUser, error) {
	phone, err := normalizePasskeyPhone(phone)
	if err != nil {
		return PasskeyLoginUser{}, err
	}
	var u PasskeyLoginUser
	err = s.DB.QueryRowContext(ctx, `
SELECT p.id,p.phone,p.profile_remark,COALESCE(p.member_id,0),COALESCE(m.name,''),pu.user_handle
FROM passkey_login_identities p
JOIN passkey_login_users pu ON pu.identity_id=p.id AND pu.rp_id=?
LEFT JOIN members m ON m.id=p.member_id
WHERE p.phone=?`, rpID, phone).Scan(&u.IdentityID, &u.Phone, &u.DisplayName, &u.MemberID, &u.MemberName, &u.UserHandle)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PasskeyLoginUser{}, errors.New("未找到该手机号对应的 Passkey 登录身份")
		}
		return PasskeyLoginUser{}, err
	}
	u.Credentials, err = s.passkeyLoginCredentials(ctx, u.IdentityID, rpID)
	if err != nil {
		return PasskeyLoginUser{}, err
	}
	if len(u.Credentials) == 0 {
		return PasskeyLoginUser{}, errors.New("该登录身份没有可用 Passkey")
	}
	return u, nil
}

func (s *Store) PasskeyLoginUserByID(ctx context.Context, identityID int64, rpID string) (PasskeyLoginUser, error) {
	var u PasskeyLoginUser
	err := s.DB.QueryRowContext(ctx, `
SELECT p.id,p.phone,p.profile_remark,COALESCE(p.member_id,0),COALESCE(m.name,''),pu.user_handle
FROM passkey_login_identities p
JOIN passkey_login_users pu ON pu.identity_id=p.id AND pu.rp_id=?
LEFT JOIN members m ON m.id=p.member_id
WHERE p.id=?`, rpID, identityID).Scan(&u.IdentityID, &u.Phone, &u.DisplayName, &u.MemberID, &u.MemberName, &u.UserHandle)
	if err != nil {
		return PasskeyLoginUser{}, err
	}
	u.Credentials, err = s.passkeyLoginCredentials(ctx, identityID, rpID)
	return u, err
}

func (s *Store) passkeyLoginCredentials(ctx context.Context, identityID int64, rpID string) ([]webauthn.Credential, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT credential_json,flags FROM passkey_login_credentials WHERE identity_id=? AND rp_id=? ORDER BY id`, identityID, rpID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []webauthn.Credential
	for rows.Next() {
		var raw string
		var flags int
		if err := rows.Scan(&raw, &flags); err != nil {
			return nil, err
		}
		var credential webauthn.Credential
		if err := json.Unmarshal([]byte(raw), &credential); err != nil {
			return nil, fmt.Errorf("解析 Passkey 凭据失败: %w", err)
		}
		credential.Flags = webauthn.NewCredentialFlags(protocol.AuthenticatorFlags(flags))
		out = append(out, credential)
	}
	return out, rows.Err()
}

func marshalPasskeyLoginCredential(credential *webauthn.Credential) (string, int, error) {
	if credential == nil || len(credential.ID) == 0 {
		return "", 0, errors.New("Passkey 凭据为空")
	}
	stored := *credential
	stored.Attestation = webauthn.CredentialAttestation{}
	raw, err := json.Marshal(&stored)
	if err != nil {
		return "", 0, err
	}
	return string(raw), int(credentialFlags(credential)), nil
}

func (s *Store) CreatePasskeyLoginIdentity(ctx context.Context, phone, remark, rpID string, userHandle []byte, credential *webauthn.Credential) (int64, error) {
	phone, err := normalizePasskeyPhone(phone)
	if err != nil {
		return 0, err
	}
	remark, err = normalizePasskeyRemark(remark)
	if err != nil {
		return 0, err
	}
	if len(userHandle) != 32 {
		return 0, errors.New("Passkey user handle 无效")
	}
	raw, flags, err := marshalPasskeyLoginCredential(credential)
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO passkey_login_identities(phone,profile_remark,created_at,updated_at) VALUES(?,?,?,?)`, phone, remark, now(), now())
	if err != nil {
		return 0, errors.New("该手机号已经创建过 Passkey 登录身份，请使用“找回已有身份”")
	}
	identityID, _ := res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `INSERT INTO passkey_login_users(identity_id,rp_id,user_handle,created_at) VALUES(?,?,?,?)`, identityID, rpID, userHandle, now()); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO passkey_login_credentials(identity_id,rp_id,credential_id,credential_json,flags,remark,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?)`, identityID, rpID, credential.ID, raw, flags, remark, now(), now()); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return identityID, nil
}

func (s *Store) SavePasskeyLoginCredential(ctx context.Context, identityID int64, rpID, remark string, credential *webauthn.Credential) error {
	remark, err := normalizePasskeyRemark(remark)
	if err != nil {
		return err
	}
	raw, flags, err := marshalPasskeyLoginCredential(credential)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
INSERT INTO passkey_login_credentials(identity_id,rp_id,credential_id,credential_json,flags,remark,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?)`, identityID, rpID, credential.ID, raw, flags, remark, now(), now())
	return err
}

func (s *Store) UpdatePasskeyLoginCredentialAfterLogin(ctx context.Context, identityID int64, rpID string, credential *webauthn.Credential) error {
	raw, flags, err := marshalPasskeyLoginCredential(credential)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `
UPDATE passkey_login_credentials
SET credential_json=?,flags=?,last_used_at=?,updated_at=?
WHERE identity_id=? AND rp_id=? AND credential_id=?`, raw, flags, now(), now(), identityID, rpID, credential.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("Passkey 凭据不存在")
	}
	return nil
}

func (s *Store) CreatePasskeyLoginCeremony(ctx context.Context, kind string, identityID int64, rpID, phone, remark string, session *webauthn.SessionData) (string, error) {
	if kind != "create" && kind != "login" && kind != "add" {
		return "", errors.New("未知 Passkey 登录 ceremony 类型")
	}
	if session == nil {
		return "", errors.New("Passkey ceremony session 为空")
	}
	if kind == "create" {
		var err error
		phone, err = normalizePasskeyPhone(phone)
		if err != nil {
			return "", err
		}
		remark, err = normalizePasskeyRemark(remark)
		if err != nil {
			return "", err
		}
	}
	if kind == "add" {
		var err error
		remark, err = normalizePasskeyRemark(remark)
		if err != nil {
			return "", err
		}
	}
	rawToken, hash, err := memberToken()
	if err != nil {
		return "", err
	}
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM passkey_login_ceremonies WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339Nano))
	_, err = s.DB.ExecContext(ctx, `
INSERT INTO passkey_login_ceremonies(token_hash,kind,identity_id,rp_id,phone,remark,session_json,expires_at,created_at)
VALUES(?,?,?,?,?,?,?,?,?)`, hash, kind, nullableMemberID(identityID), rpID, phone, remark, string(sessionJSON), time.Now().UTC().Add(PasskeyCeremonyTTL).Format(time.RFC3339Nano), now())
	return rawToken, err
}

func (s *Store) TakePasskeyLoginCeremony(ctx context.Context, rawToken, kind, rpID string) (PasskeyLoginCeremony, error) {
	if rawToken == "" {
		return PasskeyLoginCeremony{}, errors.New("Passkey 验证会话已失效，请重新开始")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return PasskeyLoginCeremony{}, err
	}
	defer tx.Rollback()
	var identity sql.NullInt64
	var storedKind, storedRPID, phone, remark, sessionJSON, expires string
	err = tx.QueryRowContext(ctx, `
SELECT kind,identity_id,rp_id,phone,remark,session_json,expires_at
FROM passkey_login_ceremonies WHERE token_hash=?`, memberTokenHash(rawToken)).Scan(&storedKind, &identity, &storedRPID, &phone, &remark, &sessionJSON, &expires)
	if err != nil {
		return PasskeyLoginCeremony{}, errors.New("Passkey 验证会话已失效，请重新开始")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM passkey_login_ceremonies WHERE token_hash=?`, memberTokenHash(rawToken)); err != nil {
		return PasskeyLoginCeremony{}, err
	}
	if storedKind != kind || storedRPID != rpID {
		return PasskeyLoginCeremony{}, errors.New("Passkey 验证会话与当前操作或站点不匹配")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || time.Now().UTC().After(expiresAt) {
		return PasskeyLoginCeremony{}, errors.New("Passkey 验证会话已过期，请重新开始")
	}
	var session webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionJSON), &session); err != nil {
		return PasskeyLoginCeremony{}, err
	}
	if err := tx.Commit(); err != nil {
		return PasskeyLoginCeremony{}, err
	}
	var identityID int64
	if identity.Valid {
		identityID = identity.Int64
	}
	return PasskeyLoginCeremony{Kind: storedKind, IdentityID: identityID, Phone: phone, Remark: remark, Session: session}, nil
}

func (s *Store) CreatePasskeyLoginIdentitySession(ctx context.Context, identityID int64) (string, error) {
	raw, hash, err := memberToken()
	if err != nil {
		return "", err
	}
	verified := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.DB.ExecContext(ctx, `
INSERT INTO passkey_login_sessions(token_hash,identity_id,expires_at,verified_at,created_at,last_seen_at)
VALUES(?,?,?,?,?,?)`, hash, identityID, time.Now().UTC().Add(PasskeyLoginIdentitySessionTTL).Format(time.RFC3339Nano), verified, now(), now())
	return raw, err
}

func (s *Store) PasskeyLoginIdentityFromSession(ctx context.Context, raw string) (PasskeyLoginIdentityView, bool, error) {
	if raw == "" {
		return PasskeyLoginIdentityView{}, false, sql.ErrNoRows
	}
	hash := memberTokenHash(raw)
	var v PasskeyLoginIdentityView
	var expires, verified string
	err := s.DB.QueryRowContext(ctx, `
SELECT p.id,p.phone,p.profile_remark,COALESCE(p.member_id,0),COALESCE(m.name,''),p.created_at,p.updated_at,s.expires_at,s.verified_at
FROM passkey_login_sessions s
JOIN passkey_login_identities p ON p.id=s.identity_id
LEFT JOIN members m ON m.id=p.member_id
WHERE s.token_hash=?`, hash).Scan(&v.ID, &v.Phone, &v.ProfileRemark, &v.MemberID, &v.MemberName, &v.CreatedAt, &v.UpdatedAt, &expires, &verified)
	if err != nil {
		return PasskeyLoginIdentityView{}, false, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || time.Now().UTC().After(expiresAt) {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM passkey_login_sessions WHERE token_hash=?`, hash)
		return PasskeyLoginIdentityView{}, false, sql.ErrNoRows
	}
	verifiedAt, err := time.Parse(time.RFC3339Nano, verified)
	if err != nil {
		return PasskeyLoginIdentityView{}, false, err
	}
	v.Credentials, err = s.PasskeyLoginCredentialViews(ctx, v.ID)
	if err != nil {
		return PasskeyLoginIdentityView{}, false, err
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE passkey_login_sessions SET last_seen_at=? WHERE token_hash=?`, now(), hash)
	fresh := time.Since(verifiedAt) <= PasskeyLoginIdentityFreshTTL
	return v, fresh, nil
}

func (s *Store) DeletePasskeyLoginIdentitySession(ctx context.Context, raw string) {
	if raw != "" {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM passkey_login_sessions WHERE token_hash=?`, memberTokenHash(raw))
	}
}

func (s *Store) PasskeyLoginCredentialViews(ctx context.Context, identityID int64) ([]PasskeyLoginCredentialView, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT id,rp_id,remark,created_at,COALESCE(last_used_at,'')
FROM passkey_login_credentials WHERE identity_id=? ORDER BY created_at DESC,id DESC`, identityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PasskeyLoginCredentialView
	for rows.Next() {
		var v PasskeyLoginCredentialView
		if err := rows.Scan(&v.ID, &v.RPID, &v.Remark, &v.CreatedAt, &v.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeletePasskeyLoginCredential removes the selected authenticator credential.
// If it was the identity's last credential, the now-unusable login identity is
// removed too so the phone number can create a fresh identity. The associated
// family member is a parent record and is never deleted by either operation.
func (s *Store) DeletePasskeyLoginCredential(ctx context.Context, credentialID int64) error {
	if credentialID <= 0 {
		return errors.New("Passkey 凭据不存在")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var identityID int64
	if err := tx.QueryRowContext(ctx, `SELECT identity_id FROM passkey_login_credentials WHERE id=?`, credentialID).Scan(&identityID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("Passkey 凭据不存在")
		}
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM passkey_login_credentials WHERE id=?`, credentialID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected != 1 {
		return errors.New("Passkey 凭据不存在")
	}
	var remaining int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM passkey_login_credentials WHERE identity_id=?`, identityID).Scan(&remaining); err != nil {
		return err
	}
	if remaining == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM passkey_login_identities WHERE id=?`, identityID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) AllPasskeyLoginIdentities(ctx context.Context) ([]PasskeyLoginIdentityView, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT p.id,p.phone,p.profile_remark,COALESCE(p.member_id,0),COALESCE(m.name,''),p.created_at,p.updated_at
FROM passkey_login_identities p
LEFT JOIN members m ON m.id=p.member_id
ORDER BY p.created_at DESC,p.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PasskeyLoginIdentityView
	for rows.Next() {
		var v PasskeyLoginIdentityView
		if err := rows.Scan(&v.ID, &v.Phone, &v.ProfileRemark, &v.MemberID, &v.MemberName, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Credentials, err = s.PasskeyLoginCredentialViews(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) ActiveMembersForPasskey(ctx context.Context) ([]Member, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,relation FROM members WHERE status='active' AND is_del=0 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Name, &m.Relation); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) BindPasskeyLoginIdentity(ctx context.Context, identityID, memberID int64) error {
	if memberID == 0 {
		res, err := s.DB.ExecContext(ctx, `UPDATE passkey_login_identities SET member_id=NULL,updated_at=? WHERE id=?`, now(), identityID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return errors.New("Passkey 登录身份不存在")
		}
		return nil
	}
	var exists int
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM members WHERE id=? AND status='active' AND is_del=0)`, memberID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return errors.New("选择的家族成员不存在或已停用")
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE passkey_login_identities SET member_id=?,updated_at=? WHERE id=?`, memberID, now(), identityID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("Passkey 登录身份不存在")
	}
	return nil
}
