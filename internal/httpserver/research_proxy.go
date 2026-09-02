package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const researchProxyMaxBytes int64 = 8 << 20

var (
	researchScriptRE      = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	researchMetaRefreshRE = regexp.MustCompile(`(?is)<meta\b[^>]*http-equiv\s*=\s*["']?refresh["']?[^>]*>`)
	researchEventAttrRE   = regexp.MustCompile(`(?is)\s+on[a-z0-9_-]+\s*=\s*("[^"]*"|'[^']*')`)
	researchURLAttrRE     = regexp.MustCompile(`(?is)(href|src|action)\s*=\s*("([^"]*)"|'([^']*)')`)
	researchCSSURLRE      = regexp.MustCompile(`(?is)url\(\s*["']?([^)'"\s]+)["']?\s*\)`)
)

func researchIPAllowed(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified()
}

func normalizeResearchTarget(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("网址不能为空")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("网址格式无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("只允许 http/https 网址")
	}
	if u.User != nil {
		return nil, errors.New("不允许在网址中携带用户名或密码")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, errors.New("不允许访问本机或局域网地址")
	}
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		return nil, errors.New("页内浏览器只允许 80/443 端口")
	}
	if ip := net.ParseIP(host); ip != nil && !researchIPAllowed(ip) {
		return nil, errors.New("不允许访问本机、局域网或保留地址")
	}
	return u, nil
}

func resolveResearchPublicHost(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !researchIPAllowed(ip) {
			return nil, errors.New("目标地址不是公网地址")
		}
		return []net.IPAddr{{IP: ip}}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New("目标域名没有可用地址")
	}
	for _, addr := range addrs {
		if !researchIPAllowed(addr.IP) {
			return nil, errors.New("目标域名解析到了非公网地址，已拒绝访问")
		}
	}
	return addrs, nil
}

func researchHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 6 * time.Second, KeepAlive: 20 * time.Second}
	transport := &http.Transport{
		Proxy:               nil,
		TLSHandshakeTimeout: 6 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addrs, err := resolveResearchPublicHost(ctx, host)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, addr := range addrs {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("重定向次数过多")
			}
			if _, err := normalizeResearchTarget(req.URL.String()); err != nil {
				return err
			}
			_, err := resolveResearchPublicHost(req.Context(), req.URL.Hostname())
			return err
		},
	}
}

func researchProxyURL(memberID int64, target string) string {
	return "/research/proxy?member=" + strconv.FormatInt(memberID, 10) + "&url=" + url.QueryEscape(target)
}

func researchRewriteReference(memberID int64, base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "blob:") || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "tel:") || strings.HasPrefix(lower, "javascript:") {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	absolute := base.ResolveReference(ref)
	if _, err := normalizeResearchTarget(absolute.String()); err != nil {
		return "#"
	}
	return researchProxyURL(memberID, absolute.String())
}

func rewriteResearchHTML(memberID int64, base *url.URL, body string) string {
	body = researchScriptRE.ReplaceAllString(body, "")
	body = researchMetaRefreshRE.ReplaceAllString(body, "")
	body = researchEventAttrRE.ReplaceAllString(body, "")
	body = researchURLAttrRE.ReplaceAllStringFunc(body, func(match string) string {
		parts := researchURLAttrRE.FindStringSubmatch(match)
		if len(parts) < 5 {
			return match
		}
		raw := parts[3]
		quote := `"`
		if raw == "" {
			raw = parts[4]
			quote = `'`
		}
		return parts[1] + "=" + quote + researchRewriteReference(memberID, base, raw) + quote
	})
	return body
}

func rewriteResearchCSS(memberID int64, base *url.URL, body string) string {
	return researchCSSURLRE.ReplaceAllStringFunc(body, func(match string) string {
		parts := researchCSSURLRE.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		rewritten := researchRewriteReference(memberID, base, parts[1])
		return `url("` + strings.ReplaceAll(rewritten, `"`, `%22`) + `")`
	})
}

func (s *Server) researchProxy(w http.ResponseWriter, r *http.Request) {
	member := currentMember(r)
	if member.ID <= 0 {
		http.Error(w, "未登录", http.StatusUnauthorized)
		return
	}
	if rawMember := strings.TrimSpace(r.URL.Query().Get("member")); rawMember != "" && rawMember != strconv.FormatInt(member.ID, 10) {
		http.Error(w, "浏览缓存成员与当前登录成员不一致", http.StatusForbidden)
		return
	}
	target, err := normalizeResearchTarget(r.URL.Query().Get("url"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := resolveResearchPublicHost(r.Context(), target.Hostname()); err != nil {
		http.Error(w, "目标地址被安全策略拒绝："+err.Error(), http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FmlySys Research/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/css,image/avif,image/webp,image/png,image/jpeg,*/*;q=0.8")
	req.Header.Set("Accept-Language", r.Header.Get("Accept-Language"))

	resp, err := researchHTTPClient().Do(req)
	if err != nil {
		http.Error(w, "境外服务器访问失败："+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, researchProxyMaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, "读取目标网页失败", http.StatusBadGateway)
		return
	}
	if int64(len(body)) > researchProxyMaxBytes {
		http.Error(w, "目标资源超过 8 MiB，页内浏览器拒绝加载", http.StatusRequestEntityTooLarge)
		return
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "private, max-age=600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Research-Target", target.String())

	switch {
	case strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml+xml"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self' data: blob:; script-src 'none'; connect-src 'none'; frame-src 'none'; object-src 'none'; style-src 'self' 'unsafe-inline' data:; img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' data: blob:; form-action 'self'; base-uri 'none'")
		body = []byte(rewriteResearchHTML(member.ID, target, string(body)))
	case strings.Contains(contentType, "text/css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		body = []byte(rewriteResearchCSS(member.ID, target, string(body)))
	default:
		if contentType == "" {
			contentType = http.DetectContentType(body)
		}
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func researchProxyDebug(target *url.URL) string {
	return fmt.Sprintf("%s://%s", target.Scheme, target.Host)
}
