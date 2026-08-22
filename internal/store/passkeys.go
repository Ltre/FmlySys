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
	"unicode"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const PasskeyCeremonyTTL = 5 * time.Minute

type PasskeyUser struct {
	MemberID    int64
	Name        string
	UserHandle  []byte
	Credentials []webauthn.Credential
}

func (u PasskeyUser) WebAuthnID() []byte                         { return u.UserHandle }
func (u PasskeyUser) WebAuthnName() string                       { return u.Name }
func (u PasskeyUser) WebAuthnDisplayName() string                { return u.Name }
func (u PasskeyUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

type PasskeyCredentialView struct {
	ID         int64
	MemberID   int64
	MemberName string
	RPID       string
	Remark     string
	CreatedAt  string
	LastUsedAt string
}

type PasskeyCeremony struct {
	MemberID int64
	Remark   string
	Session  webauthn.SessionData
}

func normalizePasskeyRemark(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("请填写 Passkey 备注，例如姓名、手机号和设备名称")
	}
	if utf8.RuneCountInString(v) > 160 {
		return "", errors.New("Passkey 备注最多 160 个字符")
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return "", errors.New("Passkey 备注不能包含控制字符")
		}
	}
	return v, nil
}

func (s *Store) PasskeyUserForMember(ctx context.Context, memberID int64, rpID string, ensureHandle bool) (PasskeyUser, error) {
	var u PasskeyUser
	u.MemberID = memberID
	err := s.DB.QueryRowContext(ctx, `SELECT name FROM members WHERE id=? AND status='active' AND is_del=0`, memberID).Scan(&u.Name)
	if err != nil {
		return PasskeyUser{}, err
	}

	err = s.DB.QueryRowContext(ctx, `SELECT user_handle FROM passkey_users WHERE member_id=? AND rp_id=?`, memberID, rpID).Scan(&u.UserHandle)
	if errors.Is(err, sql.ErrNoRows) && ensureHandle {
		candidate := make([]byte, 32)
		if _, err = rand.Read(candidate); err != nil {
			return PasskeyUser{}, err
		}
		if _, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO passkey_users(member_id,rp_id,user_handle,created_at) VALUES(?,?,?,?)`, memberID, rpID, candidate, now()); err != nil {
			return PasskeyUser{}, err
		}
		err = s.DB.QueryRowContext(ctx, `SELECT user_handle FROM passkey_users WHERE member_id=? AND rp_id=?`, memberID, rpID).Scan(&u.UserHandle)
	}
	if err != nil {
		return PasskeyUser{}, err
	}

	u.Credentials, err = s.passkeyCredentials(ctx, memberID, rpID)
	if err != nil {
		return PasskeyUser{}, err
	}
	return u, nil
}

func (s *Store) PasskeyUserByHandle(ctx context.Context, userHandle, credentialID []byte, rpID string) (PasskeyUser, error) {
	var memberID int64
	err := s.DB.QueryRowContext(ctx, `
SELECT pu.member_id
FROM passkey_users pu
JOIN members m ON m.id=pu.member_id
JOIN passkey_credentials pc ON pc.member_id=pu.member_id AND pc.rp_id=pu.rp_id
WHERE pu.rp_id=? AND pu.user_handle=? AND pc.credential_id=? AND m.status='active' AND m.is_del=0
LIMIT 1`, rpID, userHandle, credentialID).Scan(&memberID)
	if err != nil {
		return PasskeyUser{}, err
	}
	return s.PasskeyUserForMember(ctx, memberID, rpID, false)
}

func (s *Store) passkeyCredentials(ctx context.Context, memberID int64, rpID string) ([]webauthn.Credential, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT credential_json,flags FROM passkey_credentials WHERE member_id=? AND rp_id=? ORDER BY id`, memberID, rpID)
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

func credentialFlags(credential *webauthn.Credential) protocol.AuthenticatorFlags {
	var flags protocol.AuthenticatorFlags
	if credential.Flags.UserPresent {
		flags |= protocol.FlagUserPresent
	}
	if credential.Flags.UserVerified {
		flags |= protocol.FlagUserVerified
	}
	if credential.Flags.BackupEligible {
		flags |= protocol.FlagBackupEligible
	}
	if credential.Flags.BackupState {
		flags |= protocol.FlagBackupState
	}
	return flags
}

func (s *Store) SavePasskeyCredential(ctx context.Context, memberID int64, rpID, remark string, credential *webauthn.Credential) error {
	remark, err := normalizePasskeyRemark(remark)
	if err != nil {
		return err
	}
	if credential == nil || len(credential.ID) == 0 {
		return errors.New("Passkey 凭据为空")
	}
	stored := *credential
	stored.Attestation = webauthn.CredentialAttestation{}
	raw, err := json.Marshal(&stored)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
INSERT INTO passkey_credentials(member_id,rp_id,credential_id,credential_json,flags,remark,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?)`, memberID, rpID, credential.ID, string(raw), int(credentialFlags(credential)), remark, now(), now())
	return err
}

func (s *Store) UpdatePasskeyCredentialAfterLogin(ctx context.Context, rpID string, credential *webauthn.Credential) error {
	if credential == nil || len(credential.ID) == 0 {
		return errors.New("Passkey 凭据为空")
	}
	stored := *credential
	stored.Attestation = webauthn.CredentialAttestation{}
	raw, err := json.Marshal(&stored)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `
UPDATE passkey_credentials
SET credential_json=?,flags=?,last_used_at=?,updated_at=?
WHERE rp_id=? AND credential_id=?`, string(raw), int(credentialFlags(credential)), now(), now(), rpID, credential.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("Passkey 凭据不存在")
	}
	return nil
}

func (s *Store) MemberPasskeys(ctx context.Context, memberID int64) ([]PasskeyCredentialView, error) {
	return s.listPasskeys(ctx, `WHERE pc.member_id=?`, memberID)
}

func (s *Store) AllPasskeys(ctx context.Context) ([]PasskeyCredentialView, error) {
	return s.listPasskeys(ctx, ``, nil)
}

func (s *Store) listPasskeys(ctx context.Context, where string, arg any) ([]PasskeyCredentialView, error) {
	query := `
SELECT pc.id,pc.member_id,m.name,pc.rp_id,pc.remark,pc.created_at,COALESCE(pc.last_used_at,'')
FROM passkey_credentials pc
JOIN members m ON m.id=pc.member_id
` + where + ` ORDER BY m.name,pc.created_at DESC,pc.id DESC`
	var rows *sql.Rows
	var err error
	if where == "" {
		rows, err = s.DB.QueryContext(ctx, query)
	} else {
		rows, err = s.DB.QueryContext(ctx, query, arg)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PasskeyCredentialView
	for rows.Next() {
		var v PasskeyCredentialView
		if err := rows.Scan(&v.ID, &v.MemberID, &v.MemberName, &v.RPID, &v.Remark, &v.CreatedAt, &v.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) DeleteOwnPasskey(ctx context.Context, memberID, passkeyID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM passkey_credentials WHERE id=? AND member_id=?`, passkeyID, memberID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("Passkey 不存在或不属于当前成员")
	}
	return nil
}

func deletePasskeyAuthStateTx(ctx context.Context, tx *sql.Tx, memberID int64) error {
	// Keep member deletion compatible with databases/tests that predate the Passkey migration.
	for _, table := range []string{"passkey_ceremonies", "passkey_credentials", "passkey_users"} {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)`, table).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE member_id=?`, memberID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreatePasskeyCeremony(ctx context.Context, kind string, memberID int64, rpID, remark string, session *webauthn.SessionData) (string, error) {
	if kind != "register" && kind != "login" {
		return "", errors.New("未知 Passkey ceremony 类型")
	}
	if session == nil {
		return "", errors.New("Passkey ceremony session 为空")
	}
	if kind == "register" {
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
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM passkey_ceremonies WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339Nano))
	_, err = s.DB.ExecContext(ctx, `
INSERT INTO passkey_ceremonies(token_hash,kind,member_id,rp_id,session_json,remark,expires_at,created_at)
VALUES(?,?,?,?,?,?,?,?)`, hash, kind, nullableMemberID(memberID), rpID, string(sessionJSON), remark, time.Now().UTC().Add(PasskeyCeremonyTTL).Format(time.RFC3339Nano), now())
	return rawToken, err
}

func nullableMemberID(memberID int64) any {
	if memberID <= 0 {
		return nil
	}
	return memberID
}

func (s *Store) TakePasskeyCeremony(ctx context.Context, rawToken, kind, rpID string) (PasskeyCeremony, error) {
	if rawToken == "" {
		return PasskeyCeremony{}, errors.New("Passkey 验证会话已失效，请重新开始")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return PasskeyCeremony{}, err
	}
	defer tx.Rollback()
	var member sql.NullInt64
	var storedKind, storedRPID, sessionJSON, remark, expires string
	err = tx.QueryRowContext(ctx, `
SELECT kind,member_id,rp_id,session_json,remark,expires_at
FROM passkey_ceremonies WHERE token_hash=?`, memberTokenHash(rawToken)).Scan(&storedKind, &member, &storedRPID, &sessionJSON, &remark, &expires)
	if err != nil {
		return PasskeyCeremony{}, errors.New("Passkey 验证会话已失效，请重新开始")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM passkey_ceremonies WHERE token_hash=?`, memberTokenHash(rawToken)); err != nil {
		return PasskeyCeremony{}, err
	}
	if storedKind != kind || storedRPID != rpID {
		return PasskeyCeremony{}, errors.New("Passkey 验证会话与当前站点不匹配，请重新开始")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || time.Now().UTC().After(expiresAt) {
		return PasskeyCeremony{}, errors.New("Passkey 验证会话已过期，请重新开始")
	}
	var session webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionJSON), &session); err != nil {
		return PasskeyCeremony{}, err
	}
	if err := tx.Commit(); err != nil {
		return PasskeyCeremony{}, err
	}
	var memberID int64
	if member.Valid {
		memberID = member.Int64
	}
	return PasskeyCeremony{MemberID: memberID, Remark: remark, Session: session}, nil
}
