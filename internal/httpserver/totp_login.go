package httpserver

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Ltre/FmlySys/internal/adminauth"
	"github.com/Ltre/FmlySys/internal/store"
	qrcode "github.com/skip2/go-qrcode"
)

const (
	totpEnrollCookie   = "fmly_totp_enroll"
	totpEnrollPath     = "/auth/2fa"
	totpEnrollmentTTL  = 15 * time.Minute
	totpLoginWindow    = 5 * time.Minute
	totpLoginIPLimit   = 20
	totpLoginUserLimit = 10
)

type totpLoginPageView struct {
	Title    string
	Error    string
	Message  string
	Username string
	Remark   string
	Tab      string
}

type totpLoginRateBucket struct {
	windowStarted time.Time
	attempts      int
}

type totpIdentityAdminView struct {
	Title           string
	ActivePartition string
	AdminUsername   string
	Identities      []store.TOTPLoginIdentity
	Members         []store.Member
}

func (s *Server) totpLoginPage(w http.ResponseWriter, r *http.Request) {
	v := totpLoginPageView{Title: "通过 2FA 登录 FmlySys", Tab: "login"}
	if r.URL.Query().Get("tab") == "register" {
		v.Tab = "register"
	}
	switch r.URL.Query().Get("result") {
	case "registered":
		v.Tab = "login"
		v.Message = "2FA 身份已创建。请等待管理员将身份关联到你的家庭成员，关联完成后即可登录。"
	case "rate":
		v.Error = "尝试次数过多，请稍后再试。"
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if err := s.Templates.ExecuteTemplate(w, "totp-login.html", v); err != nil {
		http.Error(w, "登录页面暂时不可用", http.StatusInternalServerError)
	}
}

func (s *Server) totpRegisterQRCode(w http.ResponseWriter, r *http.Request) {
	if s.Admin == nil {
		http.Error(w, "2FA 登录暂不可用", http.StatusServiceUnavailable)
		return
	}
	username, _, err := store.NormalizeTOTPLoginUsername(r.URL.Query().Get("username"))
	if err != nil {
		http.Error(w, "用户名格式无效", http.StatusBadRequest)
		return
	}
	remark, err := normalizeTOTPLabelPart(r.URL.Query().Get("remark"), 80)
	if err != nil {
		http.Error(w, "2FA 补充备注最多 80 个字符", http.StatusBadRequest)
		return
	}
	token := cookieValue(r, totpEnrollCookie)
	secretEnc := ""
	if token != "" {
		secretEnc, err = s.Store.TOTPLoginEnrollmentSecret(r.Context(), totpTokenHash(token), time.Now())
	}
	if secretEnc == "" || err != nil {
		token, secretEnc, err = s.createTOTPEnrollment(r)
		if err != nil {
			http.Error(w, "暂时无法生成 2FA 注册二维码", http.StatusInternalServerError)
			return
		}
		setCookie(w, r, totpEnrollCookie, token, totpEnrollPath, int(totpEnrollmentTTL.Seconds()))
	}
	secret, err := s.Admin.DecryptTOTPSecret(secretEnc)
	if err != nil {
		http.Error(w, "2FA 注册状态无法读取，请刷新页面重试", http.StatusInternalServerError)
		return
	}
	account := "Fmly: " + username
	if remark != "" {
		account += " [" + remark + "]"
	}
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", "Fmly")
	params.Set("algorithm", "SHA1")
	params.Set("digits", "6")
	params.Set("period", "30")
	uri := "otpauth://totp/" + url.PathEscape(account) + "?" + params.Encode()
	png, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "暂时无法生成 2FA 注册二维码", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Fmly-TOTP-Label", url.QueryEscape(account))
	_, _ = w.Write(png)
}

func (s *Server) createTOTPEnrollment(r *http.Request) (string, string, error) {
	if s.Admin == nil {
		return "", "", errors.New("2FA encryption service unavailable")
	}
	token, err := randomToken()
	if err != nil {
		return "", "", err
	}
	secretBytes := make([]byte, 20)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)
	secretEnc, err := s.Admin.EncryptTOTPSecret(secret)
	if err != nil {
		return "", "", err
	}
	if err := s.Store.CreateTOTPLoginEnrollment(r.Context(), totpTokenHash(token), secretEnc, time.Now().Add(totpEnrollmentTTL)); err != nil {
		return "", "", err
	}
	return token, secretEnc, nil
}

func (s *Server) totpRegister(w http.ResponseWriter, r *http.Request) {
	if s.Admin == nil {
		http.Error(w, "2FA 登录暂不可用", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		s.renderTOTPRegisterError(w, r, "注册信息无效，请重试。", "", "")
		return
	}
	username, usernameKey, err := store.NormalizeTOTPLoginUsername(r.FormValue("username"))
	if err != nil {
		s.renderTOTPRegisterError(w, r, err.Error(), r.FormValue("username"), r.FormValue("remark"))
		return
	}
	remark, err := normalizeTOTPLabelPart(r.FormValue("remark"), 80)
	if err != nil {
		s.renderTOTPRegisterError(w, r, err.Error(), username, r.FormValue("remark"))
		return
	}
	token := cookieValue(r, totpEnrollCookie)
	if token == "" {
		s.renderTOTPRegisterError(w, r, "注册二维码已过期，请切换到登录后再切回注册并重新扫描。", username, remark)
		return
	}
	secretEnc, err := s.Store.TOTPLoginEnrollmentSecret(r.Context(), totpTokenHash(token), time.Now())
	if err != nil {
		s.renderTOTPRegisterError(w, r, "注册二维码已过期，请刷新二维码后重试。", username, remark)
		return
	}
	secret, err := s.Admin.DecryptTOTPSecret(secretEnc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	step, ok := adminauth.ValidateTOTP(secret, r.FormValue("code"), time.Now(), -1)
	if !ok {
		s.renderTOTPRegisterError(w, r, "2FA 动态验证码无效，请确认扫描的是当前二维码。", username, remark)
		return
	}
	if err := s.Store.CreateTOTPLoginIdentity(r.Context(), totpTokenHash(token), username, usernameKey, remark, secretEnc, step, time.Now()); err != nil {
		s.renderTOTPRegisterError(w, r, err.Error(), username, remark)
		return
	}
	clearCookie(w, r, totpEnrollCookie, totpEnrollPath)
	redirect(w, r, "/login/2fa?result=registered")
}

func (s *Server) renderTOTPRegisterError(w http.ResponseWriter, r *http.Request, message, username, remark string) {
	w.Header().Set("Cache-Control", "no-store")
	v := totpLoginPageView{Title: "通过 2FA 登录 FmlySys", Tab: "register", Error: message, Username: username, Remark: remark}
	if err := s.Templates.ExecuteTemplate(w, "totp-login.html", v); err != nil {
		http.Error(w, "注册页面暂时不可用", http.StatusInternalServerError)
	}
}

func (s *Server) totpLogin(w http.ResponseWriter, r *http.Request) {
	if s.Admin == nil {
		http.Error(w, "2FA 登录暂不可用", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		s.renderTOTPLoginError(w, r, "登录信息无效，请重试。", "")
		return
	}
	rawUsername := strings.TrimSpace(r.FormValue("username"))
	if !s.allowTOTPLoginAttempt(requestIPAddress(r), rawUsername, time.Now()) {
		redirect(w, r, "/login/2fa?result=rate")
		return
	}
	_, usernameKey, err := store.NormalizeTOTPLoginUsername(rawUsername)
	if err != nil {
		s.renderTOTPLoginError(w, r, "用户名或 2FA 动态验证码无效。", rawUsername)
		return
	}
	identity, err := s.Store.TOTPLoginIdentityByUsername(r.Context(), usernameKey)
	if err != nil {
		message := "用户名或 2FA 动态验证码无效。"
		if !errors.Is(err, sql.ErrNoRows) {
			s.fail(w, r, err)
			return
		}
		s.renderTOTPLoginError(w, r, message, rawUsername)
		return
	}
	secret, err := s.Admin.DecryptTOTPSecret(identity.SecretEnc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	step, ok := adminauth.ValidateTOTP(secret, r.FormValue("code"), time.Now(), identity.LastTOTPStep)
	if !ok {
		s.renderTOTPLoginError(w, r, "用户名或 2FA 动态验证码无效，或验证码已使用。", rawUsername)
		return
	}
	// Successful TOTP authentication creates an identity session even when the
	// administrator has not yet associated that identity with a family member.
	// The member session is established later, after the association is checked.
	s.Store.DeleteMemberSession(r.Context(), cookieValue(r, "fmly_session"))
	s.Store.DeletePasskeyLoginIdentitySession(r.Context(), cookieValue(r, passkeyIdentityCookie))
	s.Store.DeleteTOTPLoginIdentitySession(r.Context(), cookieValue(r, totpIdentityCookie))
	clearCookie(w, r, "fmly_session", "/")
	clearCookie(w, r, passkeyIdentityCookie, "/")
	clearCookie(w, r, totpIdentityCookie, "/")
	rawSession, err := s.Store.CreateTOTPLoginIdentitySession(r.Context(), identity.ID, step)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setCookie(w, r, totpIdentityCookie, rawSession, "/", int(store.TOTPLoginIdentitySessionTTL.Seconds()))
	redirect(w, r, "/login/2fa/pending")
}

func (s *Server) renderTOTPLoginError(w http.ResponseWriter, r *http.Request, message, username string) {
	w.Header().Set("Cache-Control", "no-store")
	v := totpLoginPageView{Title: "通过 2FA 登录 FmlySys", Tab: "login", Error: message, Username: username}
	if err := s.Templates.ExecuteTemplate(w, "totp-login.html", v); err != nil {
		http.Error(w, "登录页面暂时不可用", http.StatusInternalServerError)
	}
}

func (s *Server) allowTOTPLoginAttempt(ip, username string, now time.Time) bool {
	if ip == "" {
		ip = "unknown"
	}
	keys := []string{"ip:" + ip, "user:" + strings.ToLower(strings.TrimSpace(username)) + "@" + ip}
	s.totpLoginRateMu.Lock()
	defer s.totpLoginRateMu.Unlock()
	if s.totpLoginRate == nil {
		s.totpLoginRate = make(map[string]totpLoginRateBucket)
	}
	if len(s.totpLoginRate) > 4096 {
		for key, bucket := range s.totpLoginRate {
			if now.Sub(bucket.windowStarted) >= totpLoginWindow || now.Before(bucket.windowStarted) {
				delete(s.totpLoginRate, key)
			}
		}
	}
	for i, key := range keys {
		bucket := s.totpLoginRate[key]
		limit := totpLoginIPLimit
		if i == 1 {
			limit = totpLoginUserLimit
		}
		if bucket.windowStarted.IsZero() || now.Sub(bucket.windowStarted) >= totpLoginWindow || now.Before(bucket.windowStarted) {
			continue
		}
		if bucket.attempts >= limit {
			return false
		}
	}
	for _, key := range keys {
		bucket := s.totpLoginRate[key]
		if bucket.windowStarted.IsZero() || now.Sub(bucket.windowStarted) >= totpLoginWindow || now.Before(bucket.windowStarted) {
			bucket = totpLoginRateBucket{windowStarted: now}
		}
		bucket.attempts++
		s.totpLoginRate[key] = bucket
	}
	return true
}

func normalizeTOTPLabelPart(v string, maxRunes int) (string, error) {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > maxRunes {
		return "", errors.New("内容过长")
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return "", errors.New("内容包含不支持的字符")
		}
	}
	return v, nil
}

func totpTokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Server) adminTOTPLoginIdentities(w http.ResponseWriter, r *http.Request) {
	identities, err := s.Store.AllTOTPLoginIdentities(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	members, err := s.Store.ActiveMembersForPasskey(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := totpIdentityAdminView{
		Title:           "2FA 登录身份管理",
		ActivePartition: s.PM.ActiveID,
		AdminUsername:   currentAdmin(r).Username,
		Identities:      identities,
		Members:         members,
	}
	if err := s.Templates.ExecuteTemplate(w, "admin-totp-identities.html", v); err != nil {
		http.Error(w, "2FA 身份管理页面暂时不可用", http.StatusInternalServerError)
	}
}

func (s *Server) adminBindTOTPLoginIdentity(w http.ResponseWriter, r *http.Request) {
	identityID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || identityID <= 0 {
		s.fail(w, r, errors.New("2FA 登录身份 ID 无效"))
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	memberID, err := strconv.ParseInt(r.FormValue("member_id"), 10, 64)
	if err != nil || memberID < 0 {
		s.fail(w, r, errors.New("成员 ID 无效"))
		return
	}
	if err := s.Store.BindTOTPLoginIdentity(r.Context(), s.DevActorID, identityID, memberID); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/admin/2fa-identities")
}
