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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Ltre/FmlySys/internal/store"
)

const (
	wechatCodeStateCookiePrefix = "fmly_wechat_code_state_"
	wechatCodeStatePath         = "/auth/wechat/code/login"
	wechatCodeSceneTTL          = 10 * time.Minute
	wechatCodeTTL               = 5 * time.Minute
	wechatCodeCooldown          = 30 * time.Second
	wechatCodeDigits            = 8
)

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
		s.wechatCodeLoginUnavailable(w, v)
		return
	}
	scene, err := randomToken()
	if err != nil {
		s.wechatCodeLoginUnavailable(w, v)
		return
	}
	qrURL, err := s.wechatOA.TemporaryQRCode(r.Context(), scene, int(wechatCodeSceneTTL.Seconds()))
	if err != nil {
		s.wechatCodeLoginUnavailable(w, v)
		return
	}
	expires := time.Now().UTC().Add(wechatCodeSceneTTL)
	if err := s.Store.CreateWeChatCodeLoginAttempt(r.Context(), wechatTokenHash(state), wechatTokenHash(scene), expires); err != nil {
		s.wechatCodeLoginUnavailable(w, v)
		return
	}
	setCookie(w, r, wechatCodeStateCookieName(state), state, wechatCodeStatePath, int(wechatCodeSceneTTL.Seconds()))
	v.WeChatQRCodeURL = qrURL
	v.WeChatLoginState = state
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.render(w, "wechat-code-login.html", v)
}

func (s *Server) wechatCodeLoginUnavailable(w http.ResponseWriter, v view) {
	v.Error = "暂时无法生成微信登录二维码，请稍后刷新重试。"
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadGateway)
	s.render(w, "wechat-code-login.html", v)
}

func wechatCodeLoginError(key string) string {
	switch key {
	case "invalid":
		return "验证码错误、已过期或已使用，请扫描新二维码后重新获取。"
	case "unbound":
		return "该微信尚未绑定已审核的家族成员。请先使用“微信扫码登录”提交加入申请，并等待管理员审核。"
	case "service":
		return "微信身份校验暂时失败，请扫描新二维码后重试。"
	default:
		return "登录失败，请刷新二维码后重试。"
	}
}

func (s *Server) wechatCodeLogin(w http.ResponseWriter, r *http.Request) {
	if !s.Config.WeChatCodeLoginConfigured() || s.wechatOA == nil {
		http.Error(w, "微信验证码登录尚未配置", http.StatusServiceUnavailable)
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
		clearStateCookie()
		redirect(w, r, "/login/wechat-code?error=invalid")
		return
	}
	openID, err := s.Store.ConsumeWeChatCodeLoginCode(r.Context(), wechatTokenHash(state), wechatLoginCodeHash(code, s.Config.WeChatOAToken), time.Now())
	if err != nil {
		clearStateCookie()
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
		if message.Event == "subscribe" || message.Event == "SCAN" {
			scene := message.EventKey
			if strings.HasPrefix(scene, "qrscene_") {
				scene = strings.TrimPrefix(scene, "qrscene_")
			}
			claimed, err := s.Store.MarkWeChatCodeLoginScanned(r.Context(), wechatTokenHash(scene), message.FromUserName, time.Now())
			if err != nil {
				writeWeChatTextReply(w, message, "系统暂时繁忙，请稍后重新扫描二维码。")
				return
			}
			if claimed {
				writeWeChatTextReply(w, message, "扫码成功。请在公众号中输入“登录”获取验证码，再填入刚才打开的网页登录页。")
				return
			}
			writeWeChatTextReply(w, message, "二维码已过期或已经由其他微信使用，请刷新网页登录页后再扫描。")
			return
		}
	case "text":
		if strings.TrimSpace(message.Content) == "登录" {
			code, err := generateWeChatLoginCode()
			if err != nil {
				writeWeChatTextReply(w, message, "验证码生成失败，请稍后重试。")
				return
			}
			err = s.Store.IssueWeChatCodeLoginCode(r.Context(), message.FromUserName, wechatLoginCodeHash(code, s.Config.WeChatOAToken), time.Now(), time.Now().Add(wechatCodeTTL), wechatCodeCooldown)
			switch {
			case err == nil:
				writeWeChatTextReply(w, message, fmt.Sprintf("你的微信登录验证码是：%s。验证码 5 分钟内有效，且仅能用于刚才扫码的网页。", code))
			case errors.Is(err, store.ErrWeChatLoginCodeRateLimit):
				writeWeChatTextReply(w, message, "已在短时间内发送过验证码，请使用最近收到的验证码，或稍后再获取。")
			case errors.Is(err, store.ErrInvalidWeChatLoginAttempt):
				writeWeChatTextReply(w, message, "未找到有效的网页登录请求。请先打开网页登录页并扫描页面上的新二维码，再输入“登录”。")
			default:
				writeWeChatTextReply(w, message, "系统暂时繁忙，请稍后再试。")
			}
			return
		}
	}
	_, _ = io.WriteString(w, "success")
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
