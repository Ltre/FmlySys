package httpserver

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ltre/FmlySys/internal/config"
)

func TestRequestDiagnosticsRecordsRawFailureDetails(t *testing.T) {
	s := &Server{}
	handler := s.WithRequestDiagnostics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if string(body) != "code=12345678&password=private" {
			t.Errorf("request body=%q", body)
		}
		w.Header().Set("Set-Cookie", "session=private-response; HttpOnly")
		http.Error(w, "upstream unavailable: access_token=private-response", http.StatusBadGateway)
	}))
	probeID := strings.Repeat("x", 43)
	request := httptest.NewRequest(http.MethodPost, "/auth/wechat/code/login?diag="+probeID+"&secret=query-secret", strings.NewReader("code=12345678&password=private"))
	request.Host = "family.example.test"
	request.RemoteAddr = "203.0.113.8:4567"
	request.Header.Set("User-Agent", "Mobile Safari")
	request.Header.Set("Cookie", "fmly_session=private-cookie")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("CF-Ray", "abc123-SIN")
	request.Header.Set("CF-Connecting-IP", "203.0.113.9")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502", response.Code)
	}
	entries := s.recentDiagnostics()
	if len(entries) != 1 {
		t.Fatalf("captured %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Kind != "http_error" || entry.Method != http.MethodPost || entry.Path != "/auth/wechat/code/login" || entry.Status != http.StatusBadGateway {
		t.Fatalf("unexpected diagnostic entry: %+v", entry)
	}
	if entry.ProbeID != probeID || entry.Cloudflare != "abc123-SIN" || entry.Proto != "https" || entry.RemoteAddr != "203.0.113.8:4567" {
		t.Fatalf("missing correlation fields: %+v", entry)
	}
	if entry.Query != "diag="+probeID+"&secret=query-secret" || !strings.Contains(entry.URL, "secret=query-secret") {
		t.Fatalf("raw request URL/query not preserved: %+v", entry)
	}
	if entry.RequestHeaders.Get("Cookie") != "fmly_session=private-cookie" || entry.RequestHeaders.Get("CF-Connecting-IP") != "203.0.113.9" {
		t.Fatalf("raw request headers not preserved: %+v", entry.RequestHeaders)
	}
	if entry.RequestBody != "code=12345678&password=private" || entry.RequestBodyTruncated {
		t.Fatalf("raw request body=%q truncated=%v", entry.RequestBody, entry.RequestBodyTruncated)
	}
	if entry.ResponseHeaders.Get("Set-Cookie") != "session=private-response; HttpOnly" || !strings.Contains(entry.ResponseBody, "access_token=private-response") {
		t.Fatalf("raw response not preserved: headers=%v body=%q", entry.ResponseHeaders, entry.ResponseBody)
	}
}

func TestRequestDiagnosticsRecordsSuccessfulWechatCallbackButSkipsNormalSuccess(t *testing.T) {
	s := &Server{}
	handler := s.WithRequestDiagnostics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/wechat/code/callback" {
			_, _ = io.WriteString(w, "<xml><Content>验证码是 12345678</Content></xml>")
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))

	callback := httptest.NewRequest(http.MethodPost, "/auth/wechat/code/callback?signature=raw-signature&timestamp=123&nonce=raw-nonce", strings.NewReader("<xml><FromUserName>raw-openid</FromUserName></xml>"))
	callback.Header.Set("Content-Type", "application/xml")
	callbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(callbackResponse, callback)
	if callbackResponse.Code != http.StatusOK {
		t.Fatalf("callback status=%d, want 200", callbackResponse.Code)
	}

	ping := httptest.NewRequest(http.MethodGet, "/__diag/ping?diag="+strings.Repeat("p", 43), nil)
	pingResponse := httptest.NewRecorder()
	handler.ServeHTTP(pingResponse, ping)
	if pingResponse.Code != http.StatusOK {
		t.Fatalf("ping status=%d, want 200", pingResponse.Code)
	}

	entries := s.recentDiagnostics()
	if len(entries) != 1 {
		t.Fatalf("captured %d entries, want only the successful WeChat callback", len(entries))
	}
	entry := entries[0]
	if entry.Kind != "wechat_callback" || entry.Status != http.StatusOK || !strings.Contains(entry.RequestBody, "raw-openid") || !strings.Contains(entry.ResponseBody, "12345678") {
		t.Fatalf("successful callback details missing: %+v", entry)
	}
}

func TestRequestDiagnosticsRecordsSuccessfulOAuthCallback(t *testing.T) {
	s := &Server{}
	handler := s.WithRequestDiagnostics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}))
	request := httptest.NewRequest(http.MethodGet, "/auth/wechat/callback?code=oauth-secret&state=browser-state", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("callback status=%d, want 302", response.Code)
	}
	entries := s.recentDiagnostics()
	if len(entries) != 1 || entries[0].Kind != "wechat_callback" || entries[0].Status != http.StatusFound {
		t.Fatalf("successful OAuth callback was not retained: %+v", entries)
	}
	if entries[0].Query != "code=oauth-secret&state=browser-state" || entries[0].ResponseHeaders.Get("Location") != "/" {
		t.Fatalf("OAuth callback details missing: %+v", entries[0])
	}
}

func TestRequestDiagnosticsMarksLongURLAndQueryAsTruncated(t *testing.T) {
	s := &Server{}
	handler := s.WithRequestDiagnostics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed", http.StatusBadGateway)
	}))
	request := httptest.NewRequest(http.MethodGet, "/login/wechat-code?value="+strings.Repeat("x", diagnosticDetailLimit+1), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	entries := s.recentDiagnostics()
	if len(entries) != 1 || len(entries[0].Query) != diagnosticDetailLimit || !entries[0].QueryTruncated || !entries[0].URLTruncated {
		t.Fatalf("long URL/query truncation not marked: %+v", entries)
	}
}

func TestRecordWechatCodeLoginProblemKeepsRawError(t *testing.T) {
	s := &Server{}
	request := httptest.NewRequest(http.MethodGet, "/login/wechat-code?diag="+strings.Repeat("d", 43)+"&full=query", nil)
	err := errors.New(`微信公众号接口请求失败：Get "https://api.weixin.qq.com/cgi-bin/token?secret=private-secret&access_token=private-token": dial timeout`)
	s.recordWechatCodeLoginProblem(request, err, "fallback")

	entries := s.recentDiagnostics()
	if len(entries) != 1 || entries[0].Kind != "error" {
		t.Fatalf("unexpected diagnostic entries: %+v", entries)
	}
	entry := entries[0]
	if !strings.Contains(entry.Message, "private-secret") || !strings.Contains(entry.Message, "private-token") || entry.Query == "" {
		t.Fatalf("raw error detail was not retained: %+v", entry)
	}
}

func TestDiagnosticPingReturnsOriginMarker(t *testing.T) {
	s := &Server{mux: http.NewServeMux()}
	s.routes()
	request := httptest.NewRequest(http.MethodGet, "/__diag/ping", nil)
	response := httptest.NewRecorder()
	s.mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("X-FmlySys-Origin") != "ok" || response.Body.String() != "FmlySys origin reachable\n" {
		t.Fatalf("unexpected origin probe response: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
}

func TestNewParsesDeveloperDiagnosticsTemplate(t *testing.T) {
	s, err := New(nil, nil, nil, config.Config{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if s.Templates.Lookup("admin-developer.html") == nil {
		t.Fatal("developer diagnostics template was not parsed")
	}
}
