package httpserver

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/hkdf"
)

const medicationVAPIDKeyFile = "medication-vapid-private.pem"

func medicationVAPIDKeyPath(dataDir string) string {
	return filepath.Join(dataDir, medicationVAPIDKeyFile)
}

func loadOrCreateMedicationVAPIDKey(dataDir string) (*ecdsa.PrivateKey, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}
	path := medicationVAPIDKeyPath(dataDir)
	if raw, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(raw)
		if block == nil {
			return nil, errors.New("VAPID 私钥文件格式无效")
		}
		keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		key, ok := keyAny.(*ecdsa.PrivateKey)
		if !ok || key.Curve != elliptic.P256() {
			return nil, errors.New("VAPID 私钥必须是 P-256 ECDSA")
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return loadOrCreateMedicationVAPIDKey(dataDir)
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err != nil {
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

func medicationVAPIDPublicKeyRaw(key *ecdsa.PrivateKey) []byte {
	return elliptic.Marshal(elliptic.P256(), key.PublicKey.X, key.PublicKey.Y)
}

func medicationVAPIDPublicKeyBase64(key *ecdsa.PrivateKey) string {
	return base64.RawURLEncoding.EncodeToString(medicationVAPIDPublicKeyRaw(key))
}

func hkdfBytes(secret, salt, info []byte, n int) ([]byte, error) {
	r := hkdf.New(sha256.New, secret, salt, info)
	out := make([]byte, n)
	_, err := io.ReadFull(r, out)
	return out, err
}

func encryptWebPushAES128GCM(clientPublic, authSecret, payload []byte) ([]byte, error) {
	if len(clientPublic) != 65 || clientPublic[0] != 4 {
		return nil, errors.New("PWA p256dh 公钥格式无效")
	}
	curve := ecdh.P256()
	clientKey, err := curve.NewPublicKey(clientPublic)
	if err != nil {
		return nil, fmt.Errorf("PWA p256dh 公钥无效: %w", err)
	}
	serverKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := serverKey.ECDH(clientKey)
	if err != nil {
		return nil, err
	}
	serverPublic := serverKey.PublicKey().Bytes()
	keyInfo := append([]byte("WebPush: info\x00"), clientPublic...)
	keyInfo = append(keyInfo, serverPublic...)
	ikm, err := hkdfBytes(shared, authSecret, keyInfo, 32)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	cek, err := hkdfBytes(ikm, salt, []byte("Content-Encoding: aes128gcm\x00"), 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdfBytes(ikm, salt, []byte("Content-Encoding: nonce\x00"), 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain := append(append([]byte(nil), payload...), 0x02)
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	const recordSize = uint32(4096)
	body := make([]byte, 0, 16+4+1+len(serverPublic)+len(ciphertext))
	body = append(body, salt...)
	var rs [4]byte
	binary.BigEndian.PutUint32(rs[:], recordSize)
	body = append(body, rs[:]...)
	body = append(body, byte(len(serverPublic)))
	body = append(body, serverPublic...)
	body = append(body, ciphertext...)
	return body, nil
}

func base64URLJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func fixedECDSASignature(r, s *big.Int, size int) []byte {
	out := make([]byte, size*2)
	r.FillBytes(out[:size])
	s.FillBytes(out[size:])
	return out
}

func vapidAuthorization(key *ecdsa.PrivateKey, endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("PWA 推送 endpoint 无效")
	}
	header, _ := base64URLJSON(map[string]any{"typ": "JWT", "alg": "ES256"})
	payload, _ := base64URLJSON(map[string]any{
		"aud": u.Scheme + "://" + u.Host,
		"exp": time.Now().Add(12 * time.Hour).Unix(),
		"sub": "mailto:fmlysys@localhost",
	})
	unsigned := header + "." + payload
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	sig := base64.RawURLEncoding.EncodeToString(fixedECDSASignature(r, s, 32))
	jwt := unsigned + "." + sig
	return "vapid t=" + jwt + ", k=" + medicationVAPIDPublicKeyBase64(key), nil
}

func sendWebPush(ctx context.Context, key *ecdsa.PrivateKey, endpoint, p256dh, auth string, payload []byte) (int, error) {
	clientPublic, err := base64.RawURLEncoding.DecodeString(p256dh)
	if err != nil {
		return 0, errors.New("PWA p256dh 编码无效")
	}
	authSecret, err := base64.RawURLEncoding.DecodeString(auth)
	if err != nil {
		return 0, errors.New("PWA auth 编码无效")
	}
	body, err := encryptWebPushAES128GCM(clientPublic, authSecret, payload)
	if err != nil {
		return 0, err
	}
	authorization, err := vapidAuthorization(key, endpoint)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "300")
	req.Header.Set("Urgency", "high")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("PWA push service returned HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}
