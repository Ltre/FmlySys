package wechat

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestOfficialAccountQRCodeAndUnionIDReuseAccessToken(t *testing.T) {
	tokenCalls := 0
	client := NewOfficialAccount("oa-app", "oa-secret")
	client.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body string
		switch req.URL.Path {
		case "/cgi-bin/token":
			tokenCalls++
			if req.Method != http.MethodGet || req.URL.Query().Get("appid") != "oa-app" || req.URL.Query().Get("secret") != "oa-secret" {
				t.Fatalf("unexpected token request: %s %s", req.Method, req.URL)
			}
			body = `{"access_token":"cached-token","expires_in":7200}`
		case "/cgi-bin/qrcode/create":
			if req.Method != http.MethodPost || req.URL.Query().Get("access_token") != "cached-token" {
				t.Fatalf("unexpected QR request: %s %s", req.Method, req.URL)
			}
			requestBody, err := io.ReadAll(req.Body)
			if err != nil || !strings.Contains(string(requestBody), `"action_name":"QR_STR_SCENE"`) || !strings.Contains(string(requestBody), `"scene_str":"scene-token"`) {
				t.Fatalf("unexpected QR request body %q err=%v", requestBody, err)
			}
			body = `{"ticket":"temporary ticket+value","expire_seconds":600,"url":"https://weixin.qq.com/q/test"}`
		case "/cgi-bin/user/info":
			if req.Method != http.MethodGet || req.URL.Query().Get("access_token") != "cached-token" || req.URL.Query().Get("openid") != "oa-openid" {
				t.Fatalf("unexpected user info request: %s %s", req.Method, req.URL)
			}
			body = `{"openid":"oa-openid","unionid":"shared-union"}`
		default:
			t.Fatalf("unexpected URL %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	})}

	qrURL, err := client.TemporaryQRCode(context.Background(), "scene-token", 600)
	if err != nil {
		t.Fatal(err)
	}
	parsedQR, err := url.Parse(qrURL)
	if err != nil || parsedQR.Query().Get("ticket") != "temporary ticket+value" {
		t.Fatalf("QR URL=%q err=%v", qrURL, err)
	}
	unionID, err := client.UnionID(context.Background(), "oa-openid")
	if err != nil || unionID != "shared-union" {
		t.Fatalf("UnionID=%q err=%v", unionID, err)
	}
	if tokenCalls != 1 {
		t.Fatalf("access token fetched %d times, want one", tokenCalls)
	}
}
