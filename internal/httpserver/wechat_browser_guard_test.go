package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWeChatBrowserGuardBlocksFunctionalPageWithMask(t *testing.T) {
	called := false
	handler := WithWeChatBrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte("private app"))
	}))
	req := httptest.NewRequest(http.MethodGet, "https://family.example.test/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 MicroMessenger/8.0.60")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if called {
		t.Fatal("protected handler must not run in WeChat")
	}
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "wechat-browser-mask") || !strings.Contains(res.Body.String(), "手机自带浏览器") {
		t.Fatalf("unexpected guard response: status=%d body=%q", res.Code, res.Body.String())
	}
}

func TestWeChatBrowserGuardRejectsWrites(t *testing.T) {
	handler := WithWeChatBrowserGuard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler must not run")
	}))
	req := httptest.NewRequest(http.MethodPost, "https://family.example.test/assets/expenses", strings.NewReader("amount=1"))
	req.Header.Set("User-Agent", "MicroMessenger")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", res.Code, http.StatusForbidden)
	}
}

func TestWeChatBrowserGuardAllowsNormalBrowsersAndStaticAssets(t *testing.T) {
	for _, tc := range []struct {
		path string
		ua   string
	}{{"/", "Mozilla/5.0 Safari/605.1"}, {"/static/app.css", "MicroMessenger/8.0"}, {"/healthz", "MicroMessenger/8.0"}} {
		called := false
		handler := WithWeChatBrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodGet, "https://family.example.test"+tc.path, nil)
		req.Header.Set("User-Agent", tc.ua)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if !called || res.Code != http.StatusNoContent {
			t.Fatalf("path=%s called=%v status=%d", tc.path, called, res.Code)
		}
	}
}
