package httpserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Ltre/FmlySys/internal/store"
)

const (
	wechatCodeStateCookiePrefix = "fmly_wechat_code_state_"
	wechatCodeStatePath         = "/auth/wechat/code/login"
	wechatCodeSceneTTL          = 10 * time.Minute // retained for legacy attempt cleanup
	wechatCodeStateTTL          = 20 * time.Minute
	wechatCodeTTL               = 5 * time.Minute
	wechatCodeCooldown          = 30 * time.Second
	wechatCodeDigits            = 8
	wechatLoginRateWindow       = 5 * time.Minute
	wechatLoginRateMax          = 20
)

type wechatLoginRateBucket struct {
	windowStarted time.Time
	attempts      int
}

type wechatCodeMessage struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	MsgType      string `xml:"MsgType"`
	Content      string `xml:"Content"`
	Event        string `xml:"Event"`
	EventKey     string `xml:"EventKey"`
}

type wechatCodeReply struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
}

func (s *Server) wechatCodeLoginPage(w http.ResponseWriter, r *http.Request) {
	if raw := cookieValue(r, "fmly_session"); raw != "" {
		if _, _, err := s.Store.MemberFromSession(r.Context(), raw); err == nil {
			redirect(w, r, "/")
			return
		}
	}
	v := s.base("微信验证码登录")
	if !s.Config.WeChatCodeLoginConfigured() || s.wechatOA == nil {
		v.Error = "微信验证码登录尚未配置，请先设置公众号 AppID、AppSecret 和至少 16 个字符的回调 Token。"
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		s.render(w, "wechat-code-login.html", v)
		return
	}
	if r.URL.Query().Get("error") != "" {
		v.Error = wechatCodeLoginError(r.URL.Query().Get("error"))
	}
	state, err := randomToken()
	if err != nil {
		s.recordWechatCodeLoginProblem(r, err, "生成登录状态失败；请检查服务器随机数源。")
		s.wechatCodeLoginUnavailable(w, v)
		return
	}
	expires := time.Now().UTC().Add(wechatCodeStateTTL)
	if err := s.Store.CreateWeChatCodeLoginBrowserState(r.Context(), wechatTokenHash(state), expires); err != nil {
		s.recordWechatCodeLoginProblem(r, err, "保存验证码登录状态失败；请检查数据库状态和磁盘空间。")
		s.wechatCodeLoginUnavailable(w, v)
		return
	}
	setCookie(w, r, wechatCodeStateCookieName(state), state, wechatCodeStatePath, int(wechatCodeStateTTL.Seconds()))
	v.WeChatQRCodeURL = configuredWeChatQRCodeURL(s.Config.WeChatOAQRCodeURL)
	v.WeChatLoginState = state
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.render(w, "wechat-code-login.html", v)
}

func (s *Server) wechatCodeLoginUnavailable(w http.ResponseWriter, v view) {
	v.Error = "暂时无法打开微信验证码登录，请稍后刷新重试。"
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadGateway)
	s.render(w, "wechat-code-login.html", v)
}

func wechatCodeLoginError(key string) string {
	switch key {
	case "invalid":
		return "验证码错误、已过期或已使用。请在公众号发送“登录”获取新验证码。"
	case "rate":
		return "提交过于频繁，请稍后再试。"
	case "unbound":
		return "该微信尚未绑定已审核的家族成员。请先使用“微信扫码登录”提交加入申请，并等待管理员审核。"
	case "service":
		return "微信身份校验暂时失败，请稍后重新提交验证码。"
	default:
		return "登录失败，请刷新页面后重新获取验证码。"
	}
}

func (s *Server) wechatCodeLogin(w http.ResponseWriter, r *http.Request) {
	if !s.Config.WeChatCodeLoginConfigured() || s.wechatOA == nil {
		http.Error(w, "微信验证码登录尚未配置", http.StatusServiceUnavailable)
		return
	}
	if !s.allowWeChatCodeLoginAttempt(requestIPAddress(r), time.Now()) {
		redirect(w, r, "/login/wechat-code?error=rate")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		redirect(w, r, "/login/wechat-code?error=invalid")
		return
	}
	state := strings.TrimSpace(r.FormValue("state"))
	if len(state) != 43 {
		redirect(w, r, "/login/wechat-code?error=invalid")
		return
	}
	cookieName := wechatCodeStateCookieName(state)
	if rawState := cookieValue(r, cookieName); rawState == "" || subtle.ConstantTimeCompare([]byte(rawState), []byte(state)) != 1 {
		redirect(w, r, "/login/wechat-code?error=invalid")
		return
	}
	clearStateCookie := func() { clearCookie(w, r, cookieName, wechatCodeStatePath) }
	code := strings.TrimSpace(r.FormValue("code"))
	if len(code) != wechatCodeDigits || !allASCIIDigits(code) {
		_, _ = s.Store.ConsumeOpenIDWeChatLoginCode(r.Context(), wechatTokenHash(state), wechatLoginCodeHash(code, s.Config.WeChatOAToken), time.Now())
		redirect(w, r, "/login/wechat-code?error=invalid")
		return
	}
	stateHash := wechatTokenHash(state)
	codeHash := wechatLoginCodeHash(code, s.Config.WeChatOAToken)
	openID, err := s.Store.ConsumeOpenIDWeChatLoginCode(r.Context(), stateHash, codeHash, time.Now())
	if isMissingOpenIDCodeTable(err) {
		// An in-flight login created before the static-code migration can still
		// finish while its legacy scene attempt remains valid.
		openID, err = s.Store.ConsumeWeChatCodeLoginCode(r.Context(), stateHash, codeHash, time.Now())
	}
	if err != nil {
		redirect(w, r, "/login/wechat-code?error=invalid")
		return
	}
	unionID, profileErr := s.wechatOA.UnionID(r.Context(), openID)
	if profileErr != nil {
		// A direct OpenID binding can still be used when the official account is
		// not linked to the same Open Platform account and therefore has no UnionID.
		unionID = ""
	}
	memberID, err := s.Store.BoundMemberForWeChatLogin(r.Context(), openID, unionID)
	if errors.Is(err, sql.ErrNoRows) {
		clearStateCookie()
		redirect(w, r, "/login?error=wechat-unbound")
		return
	}
	if err != nil {
		clearStateCookie()
		redirect(w, r, "/login/wechat-code?error=service")
		return
	}
	raw, err := s.Store.CreateMemberSession(r.Context(), memberID)
	if err != nil {
		clearStateCookie()
		redirect(w, r, "/login/wechat-code?error=service")
		return
	}
	setCookie(w, r, "fmly_session", raw, "/", int(store.MemberSessionTTL.Seconds()))
	clearStateCookie()
	redirect(w, r, "/")
}

func (s *Server) wechatCodeCallbackVerify(w http.ResponseWriter, r *http.Request) {
	if !s.Config.WeChatCodeLoginConfigured() {
		http.Error(w, "微信验证码登录尚未配置", http.StatusServiceUnavailable)
		return
	}
	if !validWeChatSignature(s.Config.WeChatOAToken, r.URL.Query().Get("signature"), r.URL.Query().Get("timestamp"), r.URL.Query().Get("nonce")) {
		http.Error(w, "invalid signature", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, r.URL.Query().Get("echostr"))
}

func (s *Server) wechatCodeCallback(w http.ResponseWriter, r *http.Request) {
	if !s.Config.WeChatCodeLoginConfigured() {
		http.Error(w, "微信验证码登录尚未配置", http.StatusServiceUnavailable)
		return
	}
	if !validWeChatSignature(s.Config.WeChatOAToken, r.URL.Query().Get("signature"), r.URL.Query().Get("timestamp"), r.URL.Query().Get("nonce")) {
		http.Error(w, "invalid signature", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var message wechatCodeMessage
	if err := xml.NewDecoder(r.Body).Decode(&message); err != nil {
		http.Error(w, "invalid message", http.StatusBadRequest)
		return
	}
	if message.FromUserName == "" || message.ToUserName == "" {
		_, _ = io.WriteString(w, "success")
		return
	}
	switch message.MsgType {
	case "event":
		if message.Event == "subscribe" {
			s.issueWeChatLoginCodeReply(w, r, message, true)
			return
		}
		if message.Event == "SCAN" {
			s.issueWeChatLoginCodeReply(w, r, message, false)
			return
		}
	case "text":
		if strings.TrimSpace(message.Content) == "登录" {
			s.issueWeChatLoginCodeReply(w, r, message, false)
			return
		}
	}
	_, _ = io.WriteString(w, "success")
}

func (s *Server) issueWeChatLoginCodeReply(w http.ResponseWriter, r *http.Request, message wechatCodeMessage, welcome bool) {
	var code string
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		code, err = generateWeChatLoginCode()
		if err != nil {
			break
		}
		err = s.Store.IssueOpenIDWeChatLoginCode(r.Context(), message.FromUserName, wechatLoginCodeHash(code, s.Config.WeChatOAToken), time.Now(), time.Now().Add(wechatCodeTTL), wechatCodeCooldown)
		if isMissingOpenIDCodeTable(err) {
			err = s.Store.IssueWeChatCodeLoginCode(r.Context(), message.FromUserName, wechatLoginCodeHash(code, s.Config.WeChatOAToken), time.Now(), time.Now().Add(wechatCodeTTL), wechatCodeCooldown)
		}
		if !errors.Is(err, store.ErrWeChatLoginCodeCollision) {
			break
		}
	}
	switch {
	case err == nil:
		intro := ""
		if welcome {
			intro = "欢迎关注 FmlySys。"
		}
		writeWeChatTextReply(w, message, fmt.Sprintf("%s微信登录验证码：%s。5 分钟内有效且只能使用一次。请只在本人发起的 FmlySys 登录页输入，不要转发。", intro, code))
	case errors.Is(err, store.ErrWeChatLoginCodeRateLimit):
		writeWeChatTextReply(w, message, "最近刚发送过登录验证码，请使用最近收到的验证码；如果没有收到，请稍后回复“登录”重新获取。")
	default:
		writeWeChatTextReply(w, message, "暂时无法生成登录验证码，请稍后回复“登录”重试。")
	}
}

func isMissingOpenIDCodeTable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table: wechat_openid_login_codes") || strings.Contains(message, "no such table: wechat_code_login_browser_states")
}

func (s *Server) allowWeChatCodeLoginAttempt(ip string, now time.Time) bool {
	if ip == "" {
		ip = "unknown"
	}
	s.wechatLoginRateMu.Lock()
	defer s.wechatLoginRateMu.Unlock()
	if s.wechatLoginRate == nil {
		s.wechatLoginRate = make(map[string]wechatLoginRateBucket)
	}
	if len(s.wechatLoginRate) > 4096 {
		for key, bucket := range s.wechatLoginRate {
			if now.Sub(bucket.windowStarted) >= wechatLoginRateWindow {
				delete(s.wechatLoginRate, key)
			}
		}
	}
	if _, exists := s.wechatLoginRate[ip]; !exists && len(s.wechatLoginRate) >= 4096 {
		return false
	}
	bucket := s.wechatLoginRate[ip]
	if bucket.windowStarted.IsZero() || now.Sub(bucket.windowStarted) >= wechatLoginRateWindow || now.Before(bucket.windowStarted) {
		bucket = wechatLoginRateBucket{windowStarted: now}
	}
	if bucket.attempts >= wechatLoginRateMax {
		s.wechatLoginRate[ip] = bucket
		return false
	}
	bucket.attempts++
	s.wechatLoginRate[ip] = bucket
	return true
}

func configuredWeChatQRCodeURL(raw string) string {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return value
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.Scheme != "https" {
		return ""
	}
	return value
}

func writeWeChatTextReply(w http.ResponseWriter, request wechatCodeMessage, content string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	body, err := xml.Marshal(wechatCodeReply{
		ToUserName:   request.FromUserName,
		FromUserName: request.ToUserName,
		CreateTime:   time.Now().Unix(),
		MsgType:      "text",
		Content:      content,
	})
	if err != nil {
		_, _ = io.WriteString(w, "success")
		return
	}
	_, _ = w.Write(append([]byte(xml.Header), body...))
}

func validWeChatSignature(token, signature, timestamp, nonce string) bool {
	if token == "" || signature == "" || timestamp == "" || nonce == "" {
		return false
	}
	issuedAt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	delta := time.Now().Unix() - issuedAt
	if delta > int64((5*time.Minute).Seconds()) || delta < -int64((5*time.Minute).Seconds()) {
		return false
	}
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	want := hex.EncodeToString(digest[:])
	return len(want) == len(signature) && hmac.Equal([]byte(want), []byte(strings.ToLower(signature)))
}

func generateWeChatLoginCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", wechatCodeDigits, value.Int64()), nil
}

func wechatLoginCodeHash(code, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func wechatTokenHash(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func wechatCodeStateCookieName(state string) string {
	digest := sha256.Sum256([]byte(state))
	return wechatCodeStateCookiePrefix + hex.EncodeToString(digest[:16])
}

func allASCIIDigits(value string) bool {
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return value != ""
}
