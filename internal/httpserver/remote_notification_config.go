package httpserver

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	remoteNotificationKeyFile    = "medication-remote-private.pem"
	remoteNotificationConfigFile = "medication-remote.enc.json"
)

type RemoteNotificationConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type remoteNotificationEnvelope struct {
	Version    int    `json:"version"`
	WrappedKey string `json:"wrapped_key"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func remoteNotificationKeyPath(dataDir string) string {
	return filepath.Join(dataDir, remoteNotificationKeyFile)
}

func remoteNotificationConfigPath(dataDir string) string {
	return filepath.Join(dataDir, remoteNotificationConfigFile)
}

func loadOrCreateRemoteNotificationKey(dataDir string) (*rsa.PrivateKey, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}
	path := remoteNotificationKeyPath(dataDir)
	if raw, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(raw)
		if block == nil {
			return nil, errors.New("远控配置私钥文件格式无效")
		}
		keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("解析远控配置私钥失败: %w", err)
		}
		key, ok := keyAny.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("远控配置私钥不是 RSA 私钥")
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	raw := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return loadOrCreateRemoteNotificationKey(dataDir)
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return key, nil
}

func validateRemoteNotificationConfig(in RemoteNotificationConfig) (RemoteNotificationConfig, error) {
	in.Host = strings.TrimSpace(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	if in.Host == "" {
		return in, errors.New("请填写 FRP 本地 SSH 地址")
	}
	if in.Port < 1 || in.Port > 65535 {
		return in, errors.New("SSH 端口必须是 1-65535")
	}
	if in.Username == "" {
		return in, errors.New("请填写 SSH 用户名")
	}
	if in.Password == "" {
		return in, errors.New("请填写 SSH 密码")
	}
	// This channel is explicitly for the local frpc STCP listener. Restricting
	// the target to loopback avoids turning the admin field into a generic SSRF/
	// SSH pivot primitive.
	if !strings.EqualFold(in.Host, "localhost") {
		ip := net.ParseIP(strings.Trim(in.Host, "[]"))
		if ip == nil || !ip.IsLoopback() {
			return in, errors.New("FRP 本地 SSH 地址只允许 localhost/127.0.0.1/::1")
		}
	}
	return in, nil
}

func saveRemoteNotificationConfig(dataDir string, in RemoteNotificationConfig) error {
	in, err := validateRemoteNotificationConfig(in)
	if err != nil {
		return err
	}
	privateKey, err := loadOrCreateRemoteNotificationKey(dataDir)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(in)
	if err != nil {
		return err
	}
	contentKey := make([]byte, 32)
	if _, err := rand.Read(contentKey); err != nil {
		return err
	}
	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, []byte("FmlySys medication remote config v1"))
	wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &privateKey.PublicKey, contentKey, []byte("FmlySys medication remote config key"))
	if err != nil {
		return err
	}
	envelope := remoteNotificationEnvelope{
		Version:    1,
		WrappedKey: base64.RawStdEncoding.EncodeToString(wrapped),
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dataDir, ".remote-config-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	_ = tmp.Chmod(0o600)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, remoteNotificationConfigPath(dataDir)); err != nil {
		return err
	}
	_ = os.Chmod(remoteNotificationConfigPath(dataDir), 0o600)
	return nil
}

func loadRemoteNotificationConfig(dataDir string) (RemoteNotificationConfig, error) {
	raw, err := os.ReadFile(remoteNotificationConfigPath(dataDir))
	if err != nil {
		return RemoteNotificationConfig{}, err
	}
	var envelope remoteNotificationEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return RemoteNotificationConfig{}, err
	}
	if envelope.Version != 1 {
		return RemoteNotificationConfig{}, fmt.Errorf("不支持的远控配置版本: %d", envelope.Version)
	}
	wrapped, err := base64.RawStdEncoding.DecodeString(envelope.WrappedKey)
	if err != nil {
		return RemoteNotificationConfig{}, err
	}
	nonce, err := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return RemoteNotificationConfig{}, err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return RemoteNotificationConfig{}, err
	}
	privateKey, err := loadOrCreateRemoteNotificationKey(dataDir)
	if err != nil {
		return RemoteNotificationConfig{}, err
	}
	contentKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, wrapped, []byte("FmlySys medication remote config key"))
	if err != nil {
		return RemoteNotificationConfig{}, errors.New("远控配置无法解密；data 私钥可能已被替换")
	}
	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return RemoteNotificationConfig{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return RemoteNotificationConfig{}, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte("FmlySys medication remote config v1"))
	if err != nil {
		return RemoteNotificationConfig{}, errors.New("远控配置完整性校验失败")
	}
	var out RemoteNotificationConfig
	if err := json.Unmarshal(plain, &out); err != nil {
		return RemoteNotificationConfig{}, err
	}
	return validateRemoteNotificationConfig(out)
}

func shellSingleQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
}

func sendTermuxMedicationNotification(dataDir, title, body string) error {
	cfg, err := loadRemoteNotificationConfig(dataDir)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(strings.Trim(cfg.Host, "[]"), strconv.Itoa(cfg.Port))
	clientCfg := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(cfg.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // STCP listener is restricted to loopback by validation above.
		Timeout:         8 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return fmt.Errorf("连接 Termux SSH 失败: %w", err)
	}
	defer client.Close()
	cmd := "termux-notification --id fmlysys-medication --priority high --sound --title " + shellSingleQuote(title) + " --content " + shellSingleQuote(body) +
		"; termux-tts-speak " + shellSingleQuote(body)
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	if output, err := session.CombinedOutput(cmd); err != nil {
		return fmt.Errorf("Termux 通知命令失败: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}
