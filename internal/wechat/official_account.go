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
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	officialTokenEndpoint = "https://api.weixin.qq.com/cgi-bin/token"
	officialQRCEndpoint   = "https://api.weixin.qq.com/cgi-bin/qrcode/create"
	officialUserEndpoint  = "https://api.weixin.qq.com/cgi-bin/user/info"
)

type OfficialAccountClient struct {
	AppID     string
	AppSecret string
	HTTP      *http.Client
	Observer  func(context.Context, HTTPExchangeTrace)

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

const exchangeTraceLimit = 64 << 10

type HTTPExchangeTrace struct {
	At                       time.Time
	Method                   string
	URL                      string
	RequestHeaders           http.Header
	RequestHeadersTruncated  bool
	RequestBody              string
	RequestBodyTruncated     bool
	Status                   int
	ResponseHeaders          http.Header
	ResponseHeadersTruncated bool
	ResponseBody             string
	ResponseBodyTruncated    bool
	DurationMS               int64
	Error                    string
	ErrorTruncated           bool
}

type officialTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

type officialQRCodeResponse struct {
	Ticket  string `json:"ticket"`
	Expire  int    `json:"expire_seconds"`
	URL     string `json:"url"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type officialUserInfoResponse struct {
	OpenID  string `json:"openid"`
	UnionID string `json:"unionid"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type officialAPIError struct {
	code int
	msg  string
}

func (e *officialAPIError) Error() string {
	if e.msg == "" {
		return fmt.Sprintf("微信公众号接口错误 %d", e.code)
	}
	return fmt.Sprintf("微信公众号接口错误 %d：%s", e.code, e.msg)
}

func NewOfficialAccount(appID, secret string) *OfficialAccountClient {
	return &OfficialAccountClient{AppID: appID, AppSecret: secret, HTTP: &http.Client{Timeout: 12 * time.Second}}
}

func (c *OfficialAccountClient) TemporaryQRCode(ctx context.Context, scene string, expiresIn int) (string, error) {
	if len(scene) == 0 || len(scene) > 64 || expiresIn < 1 || expiresIn > 2592000 {
		return "", errors.New("微信公众号临时二维码参数无效")
	}
	body, err := json.Marshal(map[string]any{
		"expire_seconds": expiresIn,
		"action_name":    "QR_STR_SCENE",
		"action_info": map[string]any{
			"scene": map[string]string{"scene_str": scene},
		},
	})
	if err != nil {
		return "", err
	}
	var response officialQRCodeResponse
	if err := c.callWithAccessToken(ctx, http.MethodPost, officialQRCEndpoint, body, &response); err != nil {
		return "", err
	}
	if response.Ticket == "" {
		return "", errors.New("微信公众号没有返回二维码 ticket")
	}
	v := url.Values{}
	v.Set("ticket", response.Ticket)
	return "https://mp.weixin.qq.com/cgi-bin/showqrcode?" + v.Encode(), nil
}

func (c *OfficialAccountClient) UnionID(ctx context.Context, openID string) (string, error) {
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return "", errors.New("微信公众号用户 OpenID 为空")
	}
	var response officialUserInfoResponse
	v := url.Values{}
	v.Set("openid", openID)
	v.Set("lang", "zh_CN")
	if err := c.callWithAccessToken(ctx, http.MethodGet, officialUserEndpoint+"?"+v.Encode(), nil, &response); err != nil {
		return "", err
	}
	if response.OpenID != openID {
		return "", errors.New("微信公众号用户信息与扫码身份不匹配")
	}
	return strings.TrimSpace(response.UnionID), nil
}

func (c *OfficialAccountClient) callWithAccessToken(ctx context.Context, method, endpoint string, body []byte, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.AccessToken(ctx)
		if err != nil {
			return err
		}
		v, err := url.Parse(endpoint)
		if err != nil {
			return err
		}
		query := v.Query()
		query.Set("access_token", token)
		v.RawQuery = query.Encode()
		if err := c.doJSON(ctx, method, v.String(), body, out); err != nil {
			var apiErr *officialAPIError
			if attempt == 0 && errors.As(err, &apiErr) && isExpiredAccessToken(apiErr.code) {
				c.invalidateAccessToken(token)
				continue
			}
			return err
		}
		return nil
	}
	return errors.New("微信公众号 access_token 已失效")
}

func (c *OfficialAccountClient) AccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Add(30*time.Second).Before(c.tokenExpiry) {
		return c.accessToken, nil
	}
	v := url.Values{}
	v.Set("grant_type", "client_credential")
	v.Set("appid", c.AppID)
	v.Set("secret", c.AppSecret)
	endpoint := officialTokenEndpoint + "?" + v.Encode()
	var response officialTokenResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return "", err
	}
	if response.AccessToken == "" {
		return "", errors.New("微信公众号没有返回 access_token")
	}
	duration := time.Duration(response.ExpiresIn) * time.Second
	if duration <= 30*time.Second {
		duration = 30 * time.Second
	} else {
		duration -= 30 * time.Second
	}
	c.accessToken = response.AccessToken
	c.tokenExpiry = time.Now().Add(duration)
	return c.accessToken, nil
}

func (c *OfficialAccountClient) invalidateAccessToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken == token {
		c.accessToken = ""
		c.tokenExpiry = time.Time{}
	}
}

func (c *OfficialAccountClient) doJSON(ctx context.Context, method, endpoint string, body []byte, out any) error {
	started := time.Now()
	trace := HTTPExchangeTrace{At: started.UTC(), Method: method, URL: endpoint}
	if body != nil {
		trace.RequestBody, trace.RequestBodyTruncated = traceBytes(body)
	}
	var req *http.Request
	finish := func() {
		trace.DurationMS = time.Since(started).Milliseconds()
		if req != nil {
			trace.URL = req.URL.String()
			trace.RequestHeaders, trace.RequestHeadersTruncated = traceHeaders(req.Header)
		}
		if c.Observer != nil {
			c.Observer(ctx, trace)
		}
	}

	var requestBody io.Reader
	if body != nil {
		requestBody = bytes.NewReader(body)
	}
	var err error
	req, err = http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		trace.Error, trace.ErrorTruncated = traceString(err.Error())
		finish()
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		trace.Error, trace.ErrorTruncated = traceString(err.Error())
		finish()
		return fmt.Errorf("微信公众号接口请求失败：%w", err)
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
		return fmt.Errorf("微信公众号接口响应读取失败：%w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("微信公众号接口 HTTP %d", resp.StatusCode)
		trace.Error, trace.ErrorTruncated = traceString(err.Error())
		finish()
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	if err := decoder.Decode(out); err != nil {
		trace.Error, trace.ErrorTruncated = traceString(err.Error())
		finish()
		return fmt.Errorf("微信公众号接口响应解析失败：%w", err)
	}
	if apiErr := errorFromResponse(out); apiErr != nil {
		trace.Error, trace.ErrorTruncated = traceString(apiErr.Error())
		finish()
		return apiErr
	}
	finish()
	return nil
}

func traceBytes(value []byte) (string, bool) {
	if len(value) <= exchangeTraceLimit {
		return string(value), false
	}
	return string(value[:exchangeTraceLimit]), true
}

func traceString(value string) (string, bool) {
	if len(value) <= exchangeTraceLimit {
		return value, false
	}
	return value[:exchangeTraceLimit], true
}

func traceHeaders(headers http.Header) (http.Header, bool) {
	result := make(http.Header)
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	remaining := exchangeTraceLimit
	for _, key := range keys {
		for _, value := range headers[key] {
			cost := len(key) + len(value)
			if cost > remaining {
				if remaining > len(key) {
					result.Add(key, value[:remaining-len(key)])
				}
				return result, true
			}
			result.Add(key, value)
			remaining -= cost
		}
	}
	return result, false
}

func errorFromResponse(value any) error {
	var code int
	var message string
	switch response := value.(type) {
	case *tokenResponse:
		code, message = response.ErrCode, response.ErrMsg
	case *userInfoResponse:
		code, message = response.ErrCode, response.ErrMsg
	case *officialTokenResponse:
		code, message = response.ErrCode, response.ErrMsg
	case *officialQRCodeResponse:
		code, message = response.ErrCode, response.ErrMsg
	case *officialUserInfoResponse:
		code, message = response.ErrCode, response.ErrMsg
	default:
		return nil
	}
	if code == 0 {
		return nil
	}
	return &officialAPIError{code: code, msg: message}
}

func isExpiredAccessToken(code int) bool {
	return code == 40001 || code == 40014 || code == 42001
}
