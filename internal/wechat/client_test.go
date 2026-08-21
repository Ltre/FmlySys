package wechat

import (
	"net/url"
	"testing"
)

func TestLoginURLUsesRuntimeRedirectURL(t *testing.T) {
	client := New("wx-app", "secret")
	got := client.LoginURL("state-123", "https://family.example.test/auth/wechat/callback")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("appid") != "wx-app" {
		t.Fatalf("unexpected appid %q", q.Get("appid"))
	}
	if q.Get("redirect_uri") != "https://family.example.test/auth/wechat/callback" {
		t.Fatalf("unexpected redirect_uri %q", q.Get("redirect_uri"))
	}
	if q.Get("scope") != "snsapi_login" || q.Get("state") != "state-123" {
		t.Fatalf("unexpected OAuth query %v", q)
	}
}
