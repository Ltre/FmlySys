package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ltre/FmlySys/internal/config"
)

func TestRequestDiagnosticsRecordsLoginStatusWithoutQuerySecrets(t *testing.T) {
	s := &Server{}
	handler := s.WithRequestDiagnostics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	probeID := strings.Repeat("x", 43)
	request := httptest.NewRequest(http.MethodGet, "/login/wechat-code?diag="+probeID+"&secret=should-not-be-stored", nil)
	request.Host = "family.example.test"
	request.Header.Set("User-Agent", "Mobile Safari")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("CF-Ray", "abc123-SIN")
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
	if entry.Path != "/login/wechat-code" || entry.Status != http.StatusBadGateway {
		t.Fatalf("unexpected diagnostic entry: %+v", entry)
	}
	if entry.ProbeID != probeID || entry.Cloudflare != "abc123-SIN" || entry.Proto != "https" {
		t.Fatalf("missing correlation fields: %+v", entry)
	}
	if strings.Contains(entry.Path, "?") || strings.Contains(entry.Message, "should-not-be-stored") {
		t.Fatalf("diagnostic entry stored query data: %+v", entry)
	}
}

func TestSafeWechatDiagnosticErrorHidesURLTokens(t *testing.T) {
	err := errors.New(`微信公众号接口请求失败：Get "https://api.weixin.qq.com/cgi-bin/token?secret=private&access_token=private": dial timeout`)
	message := safeWechatDiagnosticError(err, "fallback")
	if strings.Contains(message, "private") || strings.Contains(message, "api.weixin.qq.com") {
		t.Fatalf("diagnostic message leaked request URL or secret: %q", message)
	}
	if !strings.Contains(message, "网络请求失败") {
		t.Fatalf("diagnostic message=%q, want network failure classification", message)
	}
}

func TestSafeWechatDiagnosticErrorKeepsPlatformErrorCode(t *testing.T) {
	message := safeWechatDiagnosticError(errors.New("微信公众号接口错误 40164：invalid ip"), "fallback")
	if !strings.Contains(message, "40164") || strings.Contains(message, "invalid ip") {
		t.Fatalf("unexpected sanitized platform error: %q", message)
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
