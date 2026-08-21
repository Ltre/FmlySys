package adminauth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	passwordIterations = 210000
	adminSessionTTL     = 12 * time.Hour
)

type Service struct {
	DB  *sql.DB
	key []byte
}

type User struct {
	ID            int64
	Username      string
	TOTPConfirmed bool
}

type Session struct {
	UserID   int64
	Username string
	Stage    string
}

func New(db *sql.DB, key []byte) *Service { return &Service{DB: db, key: key} }

func LoadMasterKey(dataDir, configured string) ([]byte, error) {
	if configured != "" {
		sum := sha256.Sum256([]byte(configured))
		return sum[:], nil
	}
	path := filepath.Join(dataDir, "system.key")
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != 32 {
			return nil, errors.New("data/system.key 长度异常，应为 32 字节")
		}
		return b, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Service) HasAdmin(ctx context.Context) (bool, error) {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM admin_users`).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Service) EnsureBootstrapAdmin(ctx context.Context, username, password string) error {
	has, err := s.HasAdmin(ctx)
	if err != nil || has || password == "" {
		return err
	}
	if len(password) < 10 {
		return errors.New("FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD 至少 10 个字符")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO admin_users(username,password_hash,totp_confirmed,last_totp_step,created_at,updated_at) VALUES(?,?,0,-1,?,?)`, username, hash, now(), now())
	return err
}

func (s *Service) VerifyPassword(ctx context.Context, username, password string) (User, error) {
	var u User
	var hash string
	var confirmed int
	err := s.DB.QueryRowContext(ctx, `SELECT id,username,password_hash,totp_confirmed FROM admin_users WHERE username=? AND status='active'`, username).Scan(&u.ID, &u.Username, &hash, &confirmed)
	if err != nil {
		return User{}, errors.New("管理员账号或密码错误")
	}
	if !verifyPassword(hash, password) {
		return User{}, errors.New("管理员账号或密码错误")
	}
	u.TOTPConfirmed = confirmed == 1
	return u, nil
}

func (s *Service) BeginSession(ctx context.Context, userID int64, stage string) (string, error) {
	raw, hash, err := newToken()
	if err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO admin_sessions(token_hash,admin_user_id,stage,expires_at,created_at,last_seen_at) VALUES(?,?,?,?,?,?)`, hash, userID, stage, time.Now().UTC().Add(adminSessionTTL).Format(time.RFC3339Nano), now(), now())
	return raw, err
}

func (s *Service) Session(ctx context.Context, raw string) (Session, error) {
	if raw == "" {
		return Session{}, sql.ErrNoRows
	}
	hash := tokenHash(raw)
	var sess Session
	var expires string
	err := s.DB.QueryRowContext(ctx, `SELECT u.id,u.username,s.stage,s.expires_at FROM admin_sessions s JOIN admin_users u ON u.id=s.admin_user_id WHERE s.token_hash=? AND u.status='active'`, hash).Scan(&sess.UserID, &sess.Username, &sess.Stage, &expires)
	if err != nil {
		return Session{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || time.Now().After(t) {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash=?`, hash)
		return Session{}, sql.ErrNoRows
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE admin_sessions SET last_seen_at=? WHERE token_hash=?`, now(), hash)
	return sess, nil
}

func (s *Service) SetSessionStage(ctx context.Context, raw, stage string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE admin_sessions SET stage=?,last_seen_at=? WHERE token_hash=?`, stage, now(), tokenHash(raw))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Service) DeleteSession(ctx context.Context, raw string) {
	if raw != "" {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash=?`, tokenHash(raw))
	}
}

func (s *Service) EnsureTOTPSecret(ctx context.Context, userID int64) (string, error) {
	var enc string
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(totp_secret_enc,'') FROM admin_users WHERE id=?`, userID).Scan(&enc)
	if err != nil {
		return "", err
	}
	if enc != "" {
		return s.decrypt(enc)
	}
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	enc, err = s.encrypt(secret)
	if err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE admin_users SET totp_secret_enc=?,updated_at=? WHERE id=?`, enc, now(), userID)
	return secret, err
}

func (s *Service) TOTPSecret(ctx context.Context, userID int64) (string, error) {
	var enc string
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(totp_secret_enc,'') FROM admin_users WHERE id=?`, userID).Scan(&enc); err != nil {
		return "", err
	}
	if enc == "" {
		return "", errors.New("尚未生成 Google Authenticator 密钥")
	}
	return s.decrypt(enc)
}

func (s *Service) ConfirmTOTP(ctx context.Context, userID int64, code string) error {
	secret, err := s.TOTPSecret(ctx, userID)
	if err != nil {
		return err
	}
	step, ok := ValidateTOTP(secret, code, time.Now(), -1)
	if !ok {
		return errors.New("Google Authenticator 验证码无效")
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE admin_users SET totp_confirmed=1,last_totp_step=?,updated_at=? WHERE id=?`, step, now(), userID)
	return err
}

func (s *Service) VerifyTOTP(ctx context.Context, userID int64, code string) error {
	secret, err := s.TOTPSecret(ctx, userID)
	if err != nil {
		return err
	}
	var last int64
	var confirmed int
	if err := s.DB.QueryRowContext(ctx, `SELECT last_totp_step,totp_confirmed FROM admin_users WHERE id=?`, userID).Scan(&last, &confirmed); err != nil {
		return err
	}
	if confirmed != 1 {
		return errors.New("Google Authenticator 尚未绑定")
	}
	step, ok := ValidateTOTP(secret, code, time.Now(), last)
	if !ok {
		return errors.New("Google Authenticator 验证码无效或已使用")
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE admin_users SET last_totp_step=?,updated_at=? WHERE id=? AND last_totp_step<?`, step, now(), userID, step)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("该验证码已使用，请等待下一组验证码")
	}
	return nil
}

func OTPAuthURI(username, secret string) string {
	issuer := "FmlySys"
	label := issuer + ":" + username
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	return "otpauth://totp/" + url.PathEscape(label) + "?" + v.Encode()
}

func ValidateTOTP(secret, code string, at time.Time, lastUsedStep int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return 0, false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	current := at.Unix() / 30
	for delta := int64(-1); delta <= 1; delta++ {
		step := current + delta
		if step <= lastUsedStep {
			continue
		}
		want, err := TOTPAt(secret, step, 6)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func TOTPAt(secret string, step int64, digits int) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, uint64(step))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(msg)
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, bin%mod), nil
}

func (s *Service) encrypt(plain string) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plain), nil)
	return base64.RawStdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func (s *Service) decrypt(encoded string) (string, error) {
	b, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(b) < gcm.NonceSize() {
		return "", errors.New("加密的 TOTP 密钥数据损坏")
	}
	plain, err := gcm.Open(nil, b[:gcm.NonceSize()], b[gcm.NonceSize():], nil)
	return string(plain), err
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2SHA256([]byte(password), salt, passwordIterations, 32)
	return "pbkdf2-sha256$" + strconv.Itoa(passwordIterations) + "$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(dk), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 10000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	hLen := 32
	blocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, blocks*hLen)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		_, _ = mac.Write(salt)
		var ctr [4]byte
		binary.BigEndian.PutUint32(ctr[:], uint32(block))
		_, _ = mac.Write(ctr[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha256.New, password)
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func newToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, tokenHash(raw), nil
}

func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
