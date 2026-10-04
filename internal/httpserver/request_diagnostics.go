package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const requestDiagnosticLimit = 100

var diagnosticIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

type requestDiagnosticEntry struct {
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     int       `json:"status"`
	DurationMS int64     `json:"duration_ms"`
	ProbeID    string    `json:"probe_id,omitempty"`
	Host       string    `json:"host,omitempty"`
	Proto      string    `json:"proto,omitempty"`
	Cloudflare string    `json:"cloudflare_ray,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	Message    string    `json:"message,omitempty"`
}

type diagnosticPageView struct {
	Title            string
	ActivePartition  string
	AdminUsername    string
	ProbeID          string
	WeChatConfigured bool
}

func (s *Server) WithRequestDiagnostics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !diagnosticPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := &diagnosticResponseWriter{ResponseWriter: w}
		defer func() {
			if recovered := recover(); recovered != nil {
				s.appendDiagnostic(requestDiagnosticEntry{
					At: time.Now().UTC(), Kind: "error", Method: r.Method, Path: r.URL.Path,
					Status: http.StatusInternalServerError, ProbeID: diagnosticProbeID(r),
					Host: r.Host, Proto: forwardedProto(r), Cloudflare: diagnosticText(r.Header.Get("CF-Ray"), 80),
					UserAgent: diagnosticText(r.UserAgent(), 180),
					Message:   "请求处理时发生 panic；完整堆栈请查看服务进程日志。",
				})
				panic(recovered)
			}

			status := wrapped.status
			if status == 0 {
				status = http.StatusOK
			}
			s.appendDiagnostic(requestDiagnosticEntry{
				At: time.Now().UTC(), Kind: "request", Method: r.Method, Path: r.URL.Path,
				Status: status, DurationMS: time.Since(start).Milliseconds(), ProbeID: diagnosticProbeID(r),
				Host: r.Host, Proto: forwardedProto(r), Cloudflare: diagnosticText(r.Header.Get("CF-Ray"), 80),
				UserAgent: diagnosticText(r.UserAgent(), 180),
			})
		}()

		next.ServeHTTP(wrapped, r)
	})
}

func diagnosticPath(path string) bool {
	switch path {
	case "/login/wechat-code", "/healthz", "/__diag/ping":
		return true
	default:
		return false
	}
}

type diagnosticResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *diagnosticResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *diagnosticResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *diagnosticResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func diagnosticProbeID(r *http.Request) string {
	id := r.URL.Query().Get("diag")
	if !diagnosticIDPattern.MatchString(id) {
		return ""
	}
	return id
}

func forwardedProto(r *http.Request) string {
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if proto == "http" || proto == "https" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return ""
}

func diagnosticText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= maxRunes {
		return value
	}
	return string([]rune(value)[:maxRunes])
}

func (s *Server) appendDiagnostic(entry requestDiagnosticEntry) {
	s.diagnosticMu.Lock()
	defer s.diagnosticMu.Unlock()
	s.diagnostics = append(s.diagnostics, entry)
	if len(s.diagnostics) > requestDiagnosticLimit {
		s.diagnostics = append([]requestDiagnosticEntry(nil), s.diagnostics[len(s.diagnostics)-requestDiagnosticLimit:]...)
	}
}

func (s *Server) recentDiagnostics() []requestDiagnosticEntry {
	s.diagnosticMu.RLock()
	defer s.diagnosticMu.RUnlock()
	entries := make([]requestDiagnosticEntry, 0, len(s.diagnostics))
	for i := len(s.diagnostics) - 1; i >= 0; i-- {
		entries = append(entries, s.diagnostics[i])
	}
	return entries
}

func (s *Server) recordWechatCodeLoginProblem(r *http.Request, err error, fallback string) {
	message := safeWechatDiagnosticError(err, fallback)
	s.appendDiagnostic(requestDiagnosticEntry{
		At: time.Now().UTC(), Kind: "error", Method: r.Method, Path: r.URL.Path,
		Status: http.StatusBadGateway, ProbeID: diagnosticProbeID(r), Host: r.Host,
		Proto: forwardedProto(r), Cloudflare: diagnosticText(r.Header.Get("CF-Ray"), 80),
		UserAgent: diagnosticText(r.UserAgent(), 180), Message: message,
	})
}

func safeWechatDiagnosticError(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	message := err.Error()
	if strings.Contains(message, "微信公众号接口请求失败") {
		return "公众号接口网络请求失败；请检查服务器出站网络、DNS 或代理。请求 URL 和密钥已隐藏。"
	}
	if strings.HasPrefix(message, "微信公众号接口 HTTP ") {
		status := strings.TrimPrefix(message, "微信公众号接口 HTTP ")
		if _, parseErr := strconv.Atoi(status); parseErr == nil {
			return "公众号接口返回 HTTP " + status + "。"
		}
	}
	if strings.HasPrefix(message, "微信公众号接口错误 ") {
		var code int
		if _, scanErr := fmt.Sscanf(message, "微信公众号接口错误 %d", &code); scanErr == nil {
			return fmt.Sprintf("公众号 API 返回错误码 %d；请核对 AppID、AppSecret、接口权限和 IP 白名单。", code)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "公众号接口请求超时；请检查服务器出站网络、DNS、代理和接口响应时间。"
	}
	return fallback
}

func (s *Server) diagnosticPing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-FmlySys-Origin", "ok")
	_, _ = w.Write([]byte("FmlySys origin reachable\n"))
}

func (s *Server) adminDeveloperCenter(w http.ResponseWriter, r *http.Request) {
	probeID, err := randomToken()
	if err != nil {
		http.Error(w, "无法生成诊断编号", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	v := diagnosticPageView{
		Title: "开发中心 · 访问诊断", ActivePartition: s.PM.ActiveID,
		AdminUsername: currentAdmin(r).Username, ProbeID: probeID,
		WeChatConfigured: s.Config.WeChatCodeLoginConfigured(),
	}
	if err := s.Templates.ExecuteTemplate(w, "admin-developer.html", v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) adminDiagnosticsJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "entries": s.recentDiagnostics()})
}
