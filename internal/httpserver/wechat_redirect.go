package httpserver

import (
	"fmt"
	"net/http"
	"strings"
	"unicode"
)

const WeChatCallbackPath = "/auth/wechat/callback"

// WeChatCallbackURL builds the OAuth redirect_uri from the request that starts
// login. The route path is fixed by code; only the externally visible scheme
// and Host come from the request, so moving the same deployment to another
// domain/IP does not require an FmlySys callback URL setting.
func WeChatCallbackURL(r *http.Request) (string, error) {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return "", fmt.Errorf("无法生成微信登录回调地址：当前请求缺少 Host")
	}
	if strings.ContainsAny(host, "/\\@") {
		return "", fmt.Errorf("无法生成微信登录回调地址：Host 格式无效")
	}
	for _, ch := range host {
		if unicode.IsSpace(ch) || unicode.IsControl(ch) {
			return "", fmt.Errorf("无法生成微信登录回调地址：Host 格式无效")
		}
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if forwarded := firstForwardedProto(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		if forwarded != "http" && forwarded != "https" {
			return "", fmt.Errorf("无法生成微信登录回调地址：X-Forwarded-Proto 必须为 http 或 https")
		}
		scheme = forwarded
	}
	return scheme + "://" + host + WeChatCallbackPath, nil
}

func firstForwardedProto(value string) string {
	if i := strings.IndexByte(value, ','); i >= 0 {
		value = value[:i]
	}
	return strings.ToLower(strings.TrimSpace(value))
}
