package httpserver

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWeChatCallbackURLUsesCurrentRequestOrigin(t *testing.T) {
	r := httptest.NewRequest("GET", "http://alpha.example.test:8080/login/wechat", nil)
	got, err := WeChatCallbackURL(r)
	if err != nil {
		t.Fatal(err)
	}
	want := "http://alpha.example.test:8080/auth/wechat/callback"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWeChatCallbackURLUsesForwardedHTTPSButCurrentHost(t *testing.T) {
	r := httptest.NewRequest("GET", "http://family.example.test/login/wechat", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "ignored.example.test")
	got, err := WeChatCallbackURL(r)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://family.example.test/auth/wechat/callback"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWeChatCallbackURLSupportsDifferentHostsWithoutConfig(t *testing.T) {
	for _, rawURL := range []string{
		"https://server-a.example.test/login/wechat",
		"https://server-b.example.test/login/wechat",
		"http://192.0.2.10:8080/login/wechat",
	} {
		r := httptest.NewRequest("GET", rawURL, nil)
		got, err := WeChatCallbackURL(r)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(got, WeChatCallbackPath) {
			t.Fatalf("unexpected callback %q", got)
		}
	}
}

func TestWeChatCallbackURLRejectsInvalidForwardedProto(t *testing.T) {
	r := httptest.NewRequest("GET", "http://family.example.test/login/wechat", nil)
	r.Header.Set("X-Forwarded-Proto", "ftp")
	if _, err := WeChatCallbackURL(r); err == nil {
		t.Fatal("expected invalid forwarded proto to fail")
	}
}
