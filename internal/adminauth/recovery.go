package adminauth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadMasterKeyForStartup prevents a missing system.key from silently replacing
// the key while encrypted administrator credentials still exist. Supplying the
// local bootstrap/reset password is treated as an explicit recovery action.
func LoadMasterKeyForStartup(dataDir, configured, recoveryPassword string) ([]byte, error) {
	if strings.TrimSpace(configured) == "" {
		keyPath := filepath.Join(dataDir, "system.key")
		if _, err := os.Stat(keyPath); errors.Is(err, os.ErrNotExist) {
			credentialsPath := filepath.Join(dataDir, CredentialsFilename)
			if _, credErr := os.Stat(credentialsPath); credErr == nil {
				if strings.TrimSpace(recoveryPassword) == "" {
					return nil, fmt.Errorf("data/system.key 缺失，但 %s 仍存在；为避免生成新密钥后锁死后台，请先恢复原 system.key，或在 data/config.env 临时设置 FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD 以执行本机恢复", credentialsPath)
				}
			} else if !errors.Is(credErr, os.ErrNotExist) {
				return nil, fmt.Errorf("检查管理员密码凭据文件失败：%w", credErr)
			}
		} else if err != nil {
			return nil, fmt.Errorf("检查 data/system.key 失败：%w", err)
		}
	}
	return LoadMasterKey(dataDir, configured)
}

// EnsureBootstrapAdminRecoverable runs normal bootstrap/migration first, then
// turns AES-GCM master-key mismatches into an explicit local recovery flow.
// Recovery is allowed only when FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD is present.
func (s *Service) EnsureBootstrapAdminRecoverable(ctx context.Context, username, password string) error {
	err := s.EnsureBootstrapAdmin(ctx, username, password)
	if err != nil {
		if !isGCMAuthenticationFailure(err) {
			return err
		}
		if strings.TrimSpace(password) == "" {
			return credentialKeyMismatchError(s.credentialsPath)
		}
		if err := s.rebuildCredentialsForCurrentKey(ctx, username, password); err != nil {
			return err
		}
	}
	return s.ensureTOTPDecryptableOrRecover(ctx, password)
}

func (s *Service) rebuildCredentialsForCurrentKey(ctx context.Context, configuredUsername, password string) error {
	if err := validateAdminPassword(password); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}

	var id int64
	var dbUsername string
	err = s.DB.QueryRowContext(ctx, `SELECT id,username FROM admin_users ORDER BY id LIMIT 1`).Scan(&id, &dbUsername)
	if errors.Is(err, sql.ErrNoRows) {
		dbUsername = strings.TrimSpace(configuredUsername)
		if dbUsername == "" {
			dbUsername = "admin"
		}
		if err := s.writeCredentials(credentialRecord{Username: dbUsername, PasswordHash: hash}); err != nil {
			return err
		}
		_, err = s.DB.ExecContext(ctx, `INSERT INTO admin_users(username,password_hash,totp_confirmed,last_totp_step,created_at,updated_at) VALUES(?,'',0,-1,?,?)`, dbUsername, now(), now())
		return err
	}
	if err != nil {
		return err
	}

	if err := s.writeCredentials(credentialRecord{Username: dbUsername, PasswordHash: hash}); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE admin_user_id=?`, id); err != nil {
		return err
	}
	return s.clearLegacyPasswordHash(ctx, id)
}

func (s *Service) ensureTOTPDecryptableOrRecover(ctx context.Context, password string) error {
	var id int64
	var enc string
	err := s.DB.QueryRowContext(ctx, `SELECT id,COALESCE(totp_secret_enc,'') FROM admin_users ORDER BY id LIMIT 1`).Scan(&id, &enc)
	if errors.Is(err, sql.ErrNoRows) || enc == "" {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := s.decrypt(enc); err == nil {
		return nil
	} else if !isGCMAuthenticationFailure(err) {
		return fmt.Errorf("Google Authenticator 加密密钥内容损坏：%w", err)
	}

	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("Google Authenticator 密钥无法由当前 data/system.key 或 FMLYSYS_MASTER_KEY 解密；请恢复原主密钥，或在 data/config.env 临时设置 FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD 后重启以重置两步验证绑定")
	}
	creds, err := s.readCredentials()
	if err != nil {
		return fmt.Errorf("执行 Google Authenticator 恢复前无法验证管理员密码凭据：%w", err)
	}
	if !verifyPassword(creds.PasswordHash, password) {
		return errors.New("FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD 与当前管理员密码凭据不一致，拒绝重置 Google Authenticator")
	}
	return s.resetTOTPBindingForKeyRecovery(ctx, id)
}

func (s *Service) resetTOTPBindingForKeyRecovery(ctx context.Context, userID int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE admin_users SET totp_secret_enc='',totp_confirmed=0,last_totp_step=-1,updated_at=? WHERE id=?`, now(), userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_sessions WHERE admin_user_id=?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func credentialKeyMismatchError(path string) error {
	return fmt.Errorf("管理员密码凭据 %s 无法由当前 data/system.key 或 FMLYSYS_MASTER_KEY 解密；请恢复原主密钥，或在 data/config.env 临时设置 FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD 后重启以重建本机凭据", path)
}

func isGCMAuthenticationFailure(err error) bool {
	return err != nil && strings.Contains(err.Error(), "cipher: message authentication failed")
}
