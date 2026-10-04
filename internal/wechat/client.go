package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	authorizeEndpoint = "https://open.weixin.qq.com/connect/qrconnect"
	tokenEndpoint     = "https://api.weixin.qq.com/sns/oauth2/access_token"
	userinfoEndpoint  = "https://api.weixin.qq.com/sns/userinfo"
)

type Client struct {
	AppID     string
	AppSecret string
	HTTP      *http.Client
	Observer  func(context.Context, HTTPExchangeTrace)
}

type Profile struct {
	OpenID   string
	UnionID  string
	Nickname string
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	OpenID      string `json:"openid"`
	UnionID     string `json:"unionid"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

type userInfoResponse struct {
	OpenID   string `json:"openid"`
	UnionID  string `json:"unionid"`
	Nickname string `json:"nickname"`
	ErrCode  int    `json:"errcode"`
	ErrMsg   string `json:"errmsg"`
}

func New(appID, secret string) *Client {
	return &Client{AppID: appID, AppSecret: secret, HTTP: &http.Client{Timeout: 12 * time.Second}}
}

func (c *Client) LoginURL(state, redirectURL string) string {
	v := url.Values{}
	v.Set("appid", c.AppID)
	v.Set("redirect_uri", redirectURL)
	v.Set("response_type", "code")
	v.Set("scope", "snsapi_login")
	v.Set("state", state)
	return authorizeEndpoint + "?" + v.Encode() + "#wechat_redirect"
}

func (c *Client) Profile(ctx context.Context, code string) (Profile, error) {
	if code == "" {
		return Profile{}, errors.New("微信授权 code 为空")
	}
	v := url.Values{}
	v.Set("appid", c.AppID)
	v.Set("secret", c.AppSecret)
	v.Set("code", code)
	v.Set("grant_type", "authorization_code")
	var tr tokenResponse
	if err := c.getJSON(ctx, tokenEndpoint+"?"+v.Encode(), &tr); err != nil {
		return Profile{}, err
	}
	if tr.ErrCode != 0 {
		return Profile{}, fmt.Errorf("微信换取登录身份失败：%d %s", tr.ErrCode, tr.ErrMsg)
	}
	if tr.AccessToken == "" || tr.OpenID == "" {
		return Profile{}, errors.New("微信登录响应缺少 access_token/openid")
	}

	uv := url.Values{}
	uv.Set("access_token", tr.AccessToken)
	uv.Set("openid", tr.OpenID)
	uv.Set("lang", "zh_CN")
	var ui userInfoResponse
	if err := c.getJSON(ctx, userinfoEndpoint+"?"+uv.Encode(), &ui); err != nil {
		return Profile{OpenID: tr.OpenID, UnionID: tr.UnionID}, nil
	}
	if ui.ErrCode != 0 {
		return Profile{OpenID: tr.OpenID, UnionID: tr.UnionID}, nil
	}
	unionID := ui.UnionID
	if unionID == "" {
		unionID = tr.UnionID
	}
	return Profile{OpenID: tr.OpenID, UnionID: unionID, Nickname: ui.Nickname}, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, out any) error {
	started := time.Now()
	trace := HTTPExchangeTrace{At: started.UTC(), Method: http.MethodGet, URL: endpoint}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		trace.Error, trace.ErrorTruncated = traceString(err.Error())
		c.observeExchange(ctx, started, req, &trace)
		return err
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, networkSnapshot := traceRequestNetwork(req, httpClient)
	finish := func() {
		c.observeExchange(ctx, started, req, &trace, networkSnapshot)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		trace.Error, trace.ErrorTruncated = traceString(err.Error())
		finish()
		return fmt.Errorf("微信接口请求失败：%w", err)
	}
	defer resp.Body.Close()
	trace.Status = resp.StatusCode
	trace.ResponseHeaders, trace.ResponseHeadersTruncated = traceHeaders(resp.Header)
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if len(responseBody) > 1<<20 {
		responseBody = responseBody[:1<<20]
		trace.ResponseBodyTruncated = true
	}
	var bodyTruncated bool
	trace.ResponseBody, bodyTruncated = traceBytes(responseBody)
	trace.ResponseBodyTruncated = trace.ResponseBodyTruncated || bodyTruncated
	if readErr != nil {
		trace.Error, trace.ErrorTruncated = traceString(readErr.Error())
		finish()
		return fmt.Errorf("微信接口响应读取失败：%w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("微信接口 HTTP %d", resp.StatusCode)
		trace.Error, trace.ErrorTruncated = traceString(err.Error())
		finish()
		return err
	}
	if err := json.NewDecoder(bytes.NewReader(responseBody)).Decode(out); err != nil {
		trace.Error, trace.ErrorTruncated = traceString(err.Error())
		finish()
		return fmt.Errorf("微信接口响应解析失败：%w", err)
	}
	if apiErr := errorFromResponse(out); apiErr != nil {
		trace.Error, trace.ErrorTruncated = traceString(apiErr.Error())
	}
	finish()
	return nil
}

func (c *Client) observeExchange(ctx context.Context, started time.Time, req *http.Request, trace *HTTPExchangeTrace, networkSnapshot ...func() HTTPNetworkTrace) {
	trace.DurationMS = time.Since(started).Milliseconds()
	if req != nil {
		trace.URL = req.URL.String()
		trace.RequestHeaders, trace.RequestHeadersTruncated = traceHeaders(req.Header)
	}
	if len(networkSnapshot) > 0 && networkSnapshot[0] != nil {
		trace.Network = networkSnapshot[0]()
	}
	if c.Observer != nil {
		c.Observer(ctx, *trace)
	}
}
