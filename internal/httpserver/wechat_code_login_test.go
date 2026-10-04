package httpserver

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Ltre/FmlySys/internal/config"
	"github.com/Ltre/FmlySys/internal/store"
	"github.com/Ltre/FmlySys/internal/wechat"
	_ "modernc.org/sqlite"
)

func TestValidWeChatSignatureRejectsInvalidAndStaleCallbacks(t *testing.T) {
	token := "callback-token-for-tests"
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := "random-nonce"
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	signature := hex.EncodeToString(digest[:])
	if !validWeChatSignature(token, signature, timestamp, nonce) {
		t.Fatal("valid callback signature was rejected")
	}
	if validWeChatSignature(token, strings.Repeat("0", len(signature)), timestamp, nonce) {
		t.Fatal("invalid callback signature was accepted")
	}
	stale := fmt.Sprintf("%d", time.Now().Add(-6*time.Minute).Unix())
	staleParts := []string{token, stale, nonce}
	sort.Strings(staleParts)
	staleDigest := sha1.Sum([]byte(strings.Join(staleParts, "")))
	if validWeChatSignature(token, hex.EncodeToString(staleDigest[:]), stale, nonce) {
		t.Fatal("stale callback timestamp was accepted")
	}
}

func TestWeChatCodeLoginPageRouteIsRegistered(t *testing.T) {
	s := &Server{mux: http.NewServeMux()}
	s.routes()
	request := httptest.NewRequest(http.MethodGet, "/login/wechat-code", nil)
	_, pattern := s.mux.Handler(request)
	if pattern != "GET /login/wechat-code" {
		t.Fatalf("route pattern=%q, want GET /login/wechat-code", pattern)
	}
}

func TestWeChatLoginCodeIsEightDigitsAndKeyed(t *testing.T) {
	code, err := generateWeChatLoginCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != wechatCodeDigits || !allASCIIDigits(code) {
		t.Fatalf("generated code=%q", code)
	}
	one := wechatLoginCodeHash(code, "callback-token-a")
	two := wechatLoginCodeHash(code, "callback-token-b")
	if one == two || one == code {
		t.Fatal("stored code hash is not keyed or still contains the raw code")
	}
}

func TestWeChatCallbackRepliesWithCodeForScannedBrowser(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE wechat_code_login_attempts (
state_hash TEXT PRIMARY KEY,scene_hash TEXT NOT NULL UNIQUE,openid TEXT NOT NULL DEFAULT '',code_hash TEXT NOT NULL DEFAULT '',
expires_at TEXT NOT NULL,code_expires_at TEXT NOT NULL DEFAULT '',code_issued_at TEXT NOT NULL DEFAULT '',scanned_at TEXT NOT NULL DEFAULT '',
failed_attempts INTEGER NOT NULL DEFAULT 0,consumed_at TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(db)
	state := "browser-state-token"
	scene := "qr-scene-token"
	now := time.Now().UTC()
	if err := st.CreateWeChatCodeLoginAttempt(ctx, wechatTokenHash(state), wechatTokenHash(scene), now.Add(wechatCodeSceneTTL)); err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.MarkWeChatCodeLoginScanned(ctx, wechatTokenHash(scene), "oa-user-openid", now); err != nil || !claimed {
		t.Fatalf("scan claimed=%v err=%v", claimed, err)
	}
	token := "callback-token-for-tests"
	s := &Server{Store: st, Config: config.Config{WeChatOAAppID: "oa-app", WeChatOAAppSecret: "oa-secret", WeChatOAToken: token}}
	values := url.Values{}
	values.Set("timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	values.Set("nonce", "request-nonce")
	values.Set("signature", weChatTestSignature(token, values.Get("timestamp"), values.Get("nonce")))
	request := httptest.NewRequest("POST", "/auth/wechat/code/callback?"+values.Encode(), strings.NewReader(`<xml><ToUserName><![CDATA[gh-account]]></ToUserName><FromUserName><![CDATA[oa-user-openid]]></FromUserName><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[登录]]></Content></xml>`))
	response := httptest.NewRecorder()
	s.wechatCodeCallback(response, request)
	if response.Code != 200 {
		t.Fatalf("callback status=%d body=%s", response.Code, response.Body.String())
	}
	var reply wechatCodeReply
	if err := xml.Unmarshal(response.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.ToUserName != "oa-user-openid" || reply.FromUserName != "gh-account" || reply.MsgType != "text" {
		t.Fatalf("unexpected reply envelope: %+v", reply)
	}
	var numericCode string
	for i := 0; i+wechatCodeDigits <= len(reply.Content); i++ {
		candidate := reply.Content[i : i+wechatCodeDigits]
		if allASCIIDigits(candidate) {
			numericCode = candidate
			break
		}
	}
	if numericCode == "" {
		t.Fatalf("reply did not include an eight-digit code: %q", reply.Content)
	}
	openID, err := st.ConsumeWeChatCodeLoginCode(ctx, wechatTokenHash(state), wechatLoginCodeHash(numericCode, token), time.Now())
	if err != nil || openID != "oa-user-openid" {
		t.Fatalf("code did not redeem for the scanned user: openID=%q err=%v", openID, err)
	}
}

func TestWeChatCodeLoginRequiresMatchingPerAttemptCookieAndCreatesMemberSession(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
CREATE TABLE wechat_code_login_attempts (
 state_hash TEXT PRIMARY KEY,scene_hash TEXT NOT NULL UNIQUE,openid TEXT NOT NULL DEFAULT '',code_hash TEXT NOT NULL DEFAULT '',
 expires_at TEXT NOT NULL,code_expires_at TEXT NOT NULL DEFAULT '',code_issued_at TEXT NOT NULL DEFAULT '',scanned_at TEXT NOT NULL DEFAULT '',
 failed_attempts INTEGER NOT NULL DEFAULT 0,consumed_at TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL
);
CREATE TABLE members(id INTEGER PRIMARY KEY,name TEXT NOT NULL,relation TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active');
CREATE TABLE wechat_identities(openid TEXT PRIMARY KEY,unionid TEXT NOT NULL DEFAULT '',member_id INTEGER);
CREATE TABLE member_sessions(token_hash TEXT PRIMARY KEY,member_id INTEGER NOT NULL,expires_at TEXT NOT NULL,created_at TEXT NOT NULL,last_seen_at TEXT NOT NULL);
INSERT INTO members(id,name,relation,status) VALUES(1,'Member A','Parent','active');
INSERT INTO wechat_identities(openid,unionid,member_id) VALUES('oa-user-openid','',1);
`)
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(db)
	state, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	scene, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := st.CreateWeChatCodeLoginAttempt(ctx, wechatTokenHash(state), wechatTokenHash(scene), now.Add(wechatCodeSceneTTL)); err != nil {
		t.Fatal(err)
	}
	if claimed, err := st.MarkWeChatCodeLoginScanned(ctx, wechatTokenHash(scene), "oa-user-openid", now); err != nil || !claimed {
		t.Fatalf("scan claimed=%v err=%v", claimed, err)
	}
	token := "callback-token-for-tests"
	code := "01234567"
	if err := st.IssueWeChatCodeLoginCode(ctx, "oa-user-openid", wechatLoginCodeHash(code, token), now, now.Add(wechatCodeTTL), wechatCodeCooldown); err != nil {
		t.Fatal(err)
	}
	client := wechat.NewOfficialAccount("oa-app", "oa-secret")
	client.HTTP = &http.Client{Transport: wechatCodeLoginTestTransport(func(req *http.Request) (*http.Response, error) {
		var body string
		switch req.URL.Path {
		case "/cgi-bin/token":
			body = `{"access_token":"test-token","expires_in":7200}`
		case "/cgi-bin/user/info":
			body = `{"openid":"oa-user-openid"}`
		default:
			return nil, fmt.Errorf("unexpected request path %q", req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	})}
	s := &Server{Store: st, Config: config.Config{WeChatOAAppID: "oa-app", WeChatOAAppSecret: "oa-secret", WeChatOAToken: token}, wechatOA: client}
	postLogin := func(cookieValue string) *httptest.ResponseRecorder {
		form := url.Values{"state": {state}, "code": {code}}
		request := httptest.NewRequest(http.MethodPost, "/auth/wechat/code/login", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.AddCookie(&http.Cookie{Name: wechatCodeStateCookieName(state), Value: cookieValue, Path: wechatCodeStatePath})
		response := httptest.NewRecorder()
		s.wechatCodeLogin(response, request)
		return response
	}
	wrongCookieResponse := postLogin("different-state")
	if wrongCookieResponse.Code != http.StatusSeeOther {
		t.Fatalf("wrong-cookie status=%d", wrongCookieResponse.Code)
	}
	var sessions int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM member_sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("wrong cookie created sessions=%d err=%v", sessions, err)
	}
	response := postLogin(state)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		t.Fatalf("login status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	var sessionCookie string
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "fmly_session" {
			sessionCookie = cookie.Value
			break
		}
	}
	if sessionCookie == "" {
		t.Fatal("successful login did not set a member session cookie")
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM member_sessions WHERE member_id=1`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("successful login sessions=%d err=%v", sessions, err)
	}
}

type wechatCodeLoginTestTransport func(*http.Request) (*http.Response, error)

func (f wechatCodeLoginTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func weChatTestSignature(token, timestamp, nonce string) string {
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(digest[:])
}
