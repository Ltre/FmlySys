package httpserver

import (
	"html"
	"net/http"
	"strings"
)

func isWeChatBrowser(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("User-Agent")), "micromessenger")
}

func weChatGuardBypass(path string) bool {
	return path == "/healthz" || strings.HasPrefix(path, "/static/")
}

// WithWeChatBrowserGuard is a temporary front door guard. It deliberately
// blocks every functional endpoint in the WeChat embedded browser until the
// in-WeChat scan/login flow is ready to be re-enabled.
func WithWeChatBrowserGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isWeChatBrowser(r) || weChatGuardBypass(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Vary", "User-Agent")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "微信内置浏览器暂时禁止使用本站功能，请改用手机自带浏览器", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(weChatGuardHTML(weChatGuardURL(r))))
	})
}

func weChatGuardURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
}

func weChatGuardHTML(rawURL string) string {
	currentURL := html.EscapeString(rawURL)
	return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><title>请使用系统浏览器打开</title><style>
*{box-sizing:border-box}html,body{margin:0;min-height:100%;font-family:Inter,"Noto Sans SC",system-ui,sans-serif;background:#eef1f5;color:#202124}.site-preview{min-height:100vh;padding:24px;filter:blur(4px);opacity:.35;pointer-events:none}.preview-card{height:150px;max-width:760px;margin:18px auto;border-radius:18px;background:#fff}.wechat-browser-mask{position:fixed;inset:0;z-index:2147483647;display:grid;place-items:center;padding:22px;background:rgba(18,24,33,.82);backdrop-filter:blur(8px)}.guard-card{width:min(520px,100%);padding:26px 22px;border-radius:20px;background:#fff;box-shadow:0 24px 80px rgba(0,0,0,.35);text-align:center}.guard-icon{width:68px;height:68px;margin:0 auto 14px;border-radius:50%;display:grid;place-items:center;background:#fff1e8;color:#d83b20;font-size:34px;font-weight:800}.guard-card h1{margin:0 0 10px;font-size:24px}.guard-card p{margin:8px 0;color:#5f6368;line-height:1.7}.guard-steps{margin:18px 0;padding:14px 16px;border-radius:12px;background:#f6f8fb;text-align:left;line-height:1.8}.guard-url{display:block;width:100%;margin-top:12px;padding:10px;border:1px solid #d9dde3;border-radius:9px;background:#fff;color:#5f6368}.copy-button{width:100%;min-height:46px;margin-top:10px;border:0;border-radius:9px;background:#1a73e8;color:#fff;font:inherit;font-weight:700}.guard-note{font-size:12px!important;color:#8a8f98!important}@media(max-width:420px){.wechat-browser-mask{padding:12px}.guard-card{padding:22px 16px}.guard-card h1{font-size:22px}}
</style></head><body><main class="site-preview" aria-hidden="true"><div class="preview-card"></div><div class="preview-card"></div></main><div class="wechat-browser-mask" role="dialog" aria-modal="true" aria-labelledby="wechat-guard-title"><section class="guard-card"><div class="guard-icon">!</div><h1 id="wechat-guard-title">请在手机自带浏览器中打开</h1><p>微信内置浏览器暂时停止使用本站功能。当前页面已被锁定，不能继续操作。</p><div class="guard-steps"><strong>打开方式</strong><br>1. 点击微信右上角“···”<br>2. 选择“在浏览器打开”<br>3. 若没有该选项，请复制链接后粘贴到系统浏览器</div><input class="guard-url" id="guard-url" value="` + currentURL + `" readonly aria-label="当前页面链接"><button class="copy-button" type="button" id="copy-url">复制当前链接</button><p class="guard-note">这是临时限制；微信扫码功能验证完成后将再开放微信内使用。</p></section></div><script>document.getElementById('copy-url').addEventListener('click',async function(){const input=document.getElementById('guard-url');try{await navigator.clipboard.writeText(input.value);this.textContent='已复制，请到系统浏览器粘贴打开'}catch(e){input.select();document.execCommand('copy');this.textContent='已复制，请到系统浏览器粘贴打开'}});</script></body></html>`
}
