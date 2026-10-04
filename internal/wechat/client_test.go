package wechat

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
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

func TestClientProfileReportsRawOAuthExchanges(t *testing.T) {
	client := New("wx-app", "wx-secret")
	var traces []HTTPExchangeTrace
	client.Observer = func(_ context.Context, trace HTTPExchangeTrace) {
		traces = append(traces, trace)
	}
	client.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body string
		switch req.URL.Path {
		case "/sns/oauth2/access_token":
			if req.URL.Query().Get("secret") != "wx-secret" || req.URL.Query().Get("code") != "oauth-code" {
				t.Fatalf("unexpected token exchange URL: %s", req.URL)
			}
			body = `{"access_token":"oauth-access-token","openid":"wx-openid","unionid":"wx-union"}`
		case "/sns/userinfo":
			if req.URL.Query().Get("access_token") != "oauth-access-token" || req.URL.Query().Get("openid") != "wx-openid" {
				t.Fatalf("unexpected user info URL: %s", req.URL)
			}
			body = `{"openid":"wx-openid","unionid":"wx-union","nickname":"Member"}`
		default:
			t.Fatalf("unexpected WeChat API URL %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}, Request: req}, nil
	})}

	profile, err := client.Profile(context.Background(), "oauth-code")
	if err != nil {
		t.Fatal(err)
	}
	if profile.OpenID != "wx-openid" || profile.UnionID != "wx-union" || profile.Nickname != "Member" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
	if len(traces) != 2 {
		t.Fatalf("captured %d OAuth API traces, want 2", len(traces))
	}
	if !strings.Contains(traces[0].URL, "secret=wx-secret") || !strings.Contains(traces[0].URL, "code=oauth-code") || !strings.Contains(traces[0].ResponseBody, "oauth-access-token") {
		t.Fatalf("OAuth token details missing: %+v", traces[0])
	}
	if traces[0].Status != http.StatusOK || !strings.Contains(traces[1].URL, "access_token=oauth-access-token") || !strings.Contains(traces[1].ResponseBody, "Member") {
		t.Fatalf("OAuth profile details missing: %+v", traces[1])
	}
}
