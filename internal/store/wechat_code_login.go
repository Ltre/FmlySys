package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidWeChatLoginAttempt = errors.New("微信验证码无效或已过期")
	ErrWeChatLoginCodeRateLimit  = errors.New("验证码请求过于频繁")
	ErrWeChatLoginCodeCollision  = errors.New("验证码冲突")
)

func (s *Store) CreateWeChatCodeLoginBrowserState(ctx context.Context, stateHash string, expiresAt time.Time) error {
	if stateHash == "" {
		return ErrInvalidWeChatLoginAttempt
	}
	createdAt := now()
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM wechat_code_login_browser_states WHERE expires_at<=?`, createdAt); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wechat_code_login_browser_states(state_hash,expires_at,created_at) VALUES(?,?,?)`, stateHash, expiresAt.UTC().Format(time.RFC3339Nano), createdAt)
	return err
}

func (s *Store) IssueOpenIDWeChatLoginCode(ctx context.Context, openID, codeHash string, issuedAt, expiresAt time.Time, cooldown time.Duration) error {
	openID = strings.TrimSpace(openID)
	if openID == "" || codeHash == "" {
		return ErrInvalidWeChatLoginAttempt
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nowTime := issuedAt.UTC()
	nowValue := nowTime.Format(time.RFC3339Nano)
	cutoff := nowTime.Add(-24 * time.Hour).Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `DELETE FROM wechat_openid_login_codes WHERE issued_at<?`, cutoff); err != nil {
		return err
	}
	var lastIssued string
	err = tx.QueryRowContext(ctx, `SELECT issued_at FROM wechat_openid_login_codes WHERE openid=?`, openID).Scan(&lastIssued)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		if last, parseErr := time.Parse(time.RFC3339Nano, lastIssued); parseErr == nil && nowTime.Sub(last) < cooldown {
			return ErrWeChatLoginCodeRateLimit
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO wechat_openid_login_codes(openid,code_hash,expires_at,issued_at,consumed_at)
VALUES(?,?,?,?, '')
ON CONFLICT(openid) DO UPDATE SET code_hash=excluded.code_hash,expires_at=excluded.expires_at,issued_at=excluded.issued_at,consumed_at=''`, openID, codeHash, expiresAt.UTC().Format(time.RFC3339Nano), nowValue)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: wechat_openid_login_codes.code_hash") {
			return ErrWeChatLoginCodeCollision
		}
		return err
	}
	return tx.Commit()
}

func (s *Store) ConsumeOpenIDWeChatLoginCode(ctx context.Context, stateHash, codeHash string, at time.Time) (string, error) {
	if stateHash == "" || codeHash == "" {
		return "", ErrInvalidWeChatLoginAttempt
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	nowTime := at.UTC()
	nowValue := nowTime.Format(time.RFC3339Nano)
	var stateExpiry string
	if err := tx.QueryRowContext(ctx, `SELECT expires_at FROM wechat_code_login_browser_states WHERE state_hash=?`, stateHash).Scan(&stateExpiry); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidWeChatLoginAttempt
		}
		return "", err
	}
	stateValidUntil, parseErr := time.Parse(time.RFC3339Nano, stateExpiry)
	if parseErr != nil || !nowTime.Before(stateValidUntil) {
		return "", ErrInvalidWeChatLoginAttempt
	}

	var openID, codeExpiry string
	err = tx.QueryRowContext(ctx, `SELECT openid,expires_at FROM wechat_openid_login_codes WHERE code_hash=? AND consumed_at=''`, codeHash).Scan(&openID, &codeExpiry)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", recordWeChatLoginBrowserFailure(ctx, tx, stateHash)
		}
		return "", err
	}
	codeValidUntil, parseErr := time.Parse(time.RFC3339Nano, codeExpiry)
	if parseErr != nil || !nowTime.Before(codeValidUntil) {
		return "", recordWeChatLoginBrowserFailure(ctx, tx, stateHash)
	}
	res, err := tx.ExecContext(ctx, `UPDATE wechat_openid_login_codes SET code_hash='',consumed_at=? WHERE openid=? AND code_hash=? AND consumed_at=''`, nowValue, openID, codeHash)
	if err != nil {
		return "", err
	}
	if n, rowsErr := res.RowsAffected(); rowsErr != nil {
		return "", rowsErr
	} else if n != 1 {
		return "", ErrInvalidWeChatLoginAttempt
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wechat_code_login_browser_states WHERE state_hash=?`, stateHash); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return openID, nil
}

func recordWeChatLoginBrowserFailure(ctx context.Context, tx *sql.Tx, stateHash string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE wechat_code_login_browser_states SET failed_attempts=failed_attempts+1 WHERE state_hash=?`, stateHash); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wechat_code_login_browser_states WHERE state_hash=? AND failed_attempts>=5`, stateHash); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return ErrInvalidWeChatLoginAttempt
}

func (s *Store) CreateWeChatCodeLoginAttempt(ctx context.Context, stateHash, sceneHash string, expiresAt time.Time) error {
	createdAt := now()
	expires := expiresAt.UTC().Format(time.RFC3339Nano)
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM wechat_code_login_attempts WHERE expires_at<=?`, createdAt); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO wechat_code_login_attempts(state_hash,scene_hash,expires_at,created_at) VALUES(?,?,?,?)`, stateHash, sceneHash, expires, createdAt)
	return err
}

func (s *Store) MarkWeChatCodeLoginScanned(ctx context.Context, sceneHash, openID string, scannedAt time.Time) (bool, error) {
	openID = strings.TrimSpace(openID)
	if sceneHash == "" || openID == "" {
		return false, nil
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE wechat_code_login_attempts
SET openid=?,scanned_at=?
WHERE scene_hash=? AND expires_at>? AND (openid='' OR openid=?)`, openID, scannedAt.UTC().Format(time.RFC3339Nano), sceneHash, now(), openID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (s *Store) IssueWeChatCodeLoginCode(ctx context.Context, openID, codeHash string, issuedAt, codeExpiresAt time.Time, cooldown time.Duration) error {
	openID = strings.TrimSpace(openID)
	if openID == "" || codeHash == "" {
		return ErrInvalidWeChatLoginAttempt
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nowTime := issuedAt.UTC()
	nowValue := nowTime.Format(time.RFC3339Nano)
	var stateHash, lastIssued string
	err = tx.QueryRowContext(ctx, `SELECT state_hash,code_issued_at FROM wechat_code_login_attempts
WHERE openid=? AND expires_at>? AND scanned_at<>'' AND consumed_at=''
ORDER BY scanned_at DESC,created_at DESC LIMIT 1`, openID, nowValue).Scan(&stateHash, &lastIssued)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidWeChatLoginAttempt
		}
		return err
	}
	if lastIssued != "" {
		if last, parseErr := time.Parse(time.RFC3339Nano, lastIssued); parseErr == nil && nowTime.Sub(last) < cooldown {
			return ErrWeChatLoginCodeRateLimit
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE wechat_code_login_attempts
SET code_hash=?,code_expires_at=?,code_issued_at=?,failed_attempts=0
WHERE state_hash=? AND openid=? AND expires_at>? AND scanned_at<>'' AND consumed_at=''`, codeHash, codeExpiresAt.UTC().Format(time.RFC3339Nano), nowValue, stateHash, openID, nowValue)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvalidWeChatLoginAttempt
	}
	return tx.Commit()
}

func constantTimeEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Store) ConsumeWeChatCodeLoginCode(ctx context.Context, stateHash, codeHash string, at time.Time) (string, error) {
	if stateHash == "" || codeHash == "" {
		return "", ErrInvalidWeChatLoginAttempt
	}
	var openID, expectedHash, attemptExpiry, codeExpiry, consumedAt string
	err := s.DB.QueryRowContext(ctx, `SELECT openid,code_hash,expires_at,code_expires_at,consumed_at
FROM wechat_code_login_attempts WHERE state_hash=?`, stateHash).Scan(&openID, &expectedHash, &attemptExpiry, &codeExpiry, &consumedAt)
	if err != nil {
		return "", ErrInvalidWeChatLoginAttempt
	}
	nowTime := at.UTC()
	validUntil, attemptErr := time.Parse(time.RFC3339Nano, attemptExpiry)
	codeValidUntil, codeErr := time.Parse(time.RFC3339Nano, codeExpiry)
	if openID == "" || consumedAt != "" || attemptErr != nil || codeErr != nil || !nowTime.Before(validUntil) || !nowTime.Before(codeValidUntil) || expectedHash == "" {
		return "", ErrInvalidWeChatLoginAttempt
	}
	if !constantTimeEqual(expectedHash, codeHash) {
		res, err := s.DB.ExecContext(ctx, `UPDATE wechat_code_login_attempts SET failed_attempts=failed_attempts+1
WHERE state_hash=? AND code_hash=? AND consumed_at='' AND failed_attempts<5 AND expires_at>? AND code_expires_at>?`, stateHash, expectedHash, nowTime.Format(time.RFC3339Nano), nowTime.Format(time.RFC3339Nano))
		if err != nil {
			return "", err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return "", err
		}
		if n == 1 {
			if _, err := s.DB.ExecContext(ctx, `DELETE FROM wechat_code_login_attempts WHERE state_hash=? AND failed_attempts>=5`, stateHash); err != nil {
				return "", err
			}
		}
		return "", ErrInvalidWeChatLoginAttempt
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE wechat_code_login_attempts SET code_hash='',code_expires_at='',consumed_at=?
WHERE state_hash=? AND code_hash=? AND consumed_at='' AND failed_attempts<5 AND expires_at>? AND code_expires_at>?`, nowTime.Format(time.RFC3339Nano), stateHash, expectedHash, nowTime.Format(time.RFC3339Nano), nowTime.Format(time.RFC3339Nano))
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrInvalidWeChatLoginAttempt
	}
	return openID, nil
}

func (s *Store) BoundMemberForWeChatLogin(ctx context.Context, openID, unionID string) (int64, error) {
	openID = strings.TrimSpace(openID)
	unionID = strings.TrimSpace(unionID)
	if openID == "" {
		return 0, sql.ErrNoRows
	}
	var bound sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT member_id FROM wechat_identities WHERE openid=?`, openID).Scan(&bound)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err == nil && bound.Valid && bound.Int64 > 0 {
		var memberID int64
		err = s.DB.QueryRowContext(ctx, `SELECT id FROM members WHERE id=? AND status='active'`, bound.Int64).Scan(&memberID)
		return memberID, err
	}
	if unionID == "" {
		return 0, sql.ErrNoRows
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT m.id FROM wechat_identities i
JOIN members m ON m.id=i.member_id
WHERE i.unionid=? AND m.status='active'
ORDER BY m.id LIMIT 2`, unionID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) != 1 {
		return 0, sql.ErrNoRows
	}
	return ids[0], nil
}
