package wechat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	AppID       string
	AppSecret   string
	RedirectURL string
	HTTP        *http.Client
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

func New(appID, secret, redirectURL string) *Client {
	return &Client{AppID: appID, AppSecret: secret, RedirectURL: redirectURL, HTTP: &http.Client{Timeout: 12 * time.Second}}
}

func (c *Client) LoginURL(state string) string {
	v := url.Values{}
	v.Set("appid", c.AppID)
	v.Set("redirect_uri", c.RedirectURL)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("微信接口请求失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("微信接口 HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("微信接口响应解析失败：%w", err)
	}
	return nil
}
