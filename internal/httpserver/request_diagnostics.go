package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Ltre/FmlySys/internal/wechat"
)

const (
	requestDiagnosticLimit = 100
	diagnosticDetailLimit  = 64 << 10
)

var diagnosticIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

type requestDiagnosticEntry struct {
	At                       time.Time   `json:"at"`
	Kind                     string      `json:"kind"`
	Method                   string      `json:"method"`
	Path                     string      `json:"path"`
	URL                      string      `json:"url,omitempty"`
	URLTruncated             bool        `json:"url_truncated,omitempty"`
	Query                    string      `json:"query,omitempty"`
	QueryTruncated           bool        `json:"query_truncated,omitempty"`
	Status                   int         `json:"status"`
	DurationMS               int64       `json:"duration_ms"`
	ProbeID                  string      `json:"probe_id,omitempty"`
	Host                     string      `json:"host,omitempty"`
	RemoteAddr               string      `json:"remote_addr,omitempty"`
	Proto                    string      `json:"proto,omitempty"`
	Cloudflare               string      `json:"cloudflare_ray,omitempty"`
	UserAgent                string      `json:"user_agent,omitempty"`
	RequestHeaders           http.Header `json:"request_headers,omitempty"`
	RequestHeadersTruncated  bool        `json:"request_headers_truncated,omitempty"`
	RequestBody              string      `json:"request_body,omitempty"`
	RequestBodyTruncated     bool        `json:"request_body_truncated,omitempty"`
	ResponseHeaders          http.Header `json:"response_headers,omitempty"`
	ResponseHeadersTruncated bool        `json:"response_headers_truncated,omitempty"`
	ResponseBody             string      `json:"response_body,omitempty"`
	ResponseBodyTruncated    bool        `json:"response_body_truncated,omitempty"`
	Message                  string      `json:"message,omitempty"`
	MessageTruncated         bool        `json:"message_truncated,omitempty"`
}

type diagnosticPageView struct {
	Title            string
	ActivePartition  string
	AdminUsername    string
	ProbeID          string
	WeChatConfigured bool
}

type diagnosticProbeContextKey struct{}

func (s *Server) WithRequestDiagnostics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !diagnosticPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		probeID := diagnosticProbeID(r)
		r = r.WithContext(context.WithValue(r.Context(), diagnosticProbeContextKey{}, probeID))

		started := time.Now()
		requestHeaders, requestHeadersTruncated := cloneDiagnosticHeaders(r.Header)
		requestBody := &diagnosticBodyCapture{}
		var diagnosticRequestBody io.ReadCloser
		if r.Body != nil && r.Body != http.NoBody {
			diagnosticRequestBody = &diagnosticReadCloser{ReadCloser: r.Body, capture: requestBody}
			r.Body = diagnosticRequestBody
		}
		responseBody := &diagnosticBodyCapture{}
		wrapped := &diagnosticResponseWriter{ResponseWriter: w, capture: responseBody}

		defer func() {
			if diagnosticRequestBody != nil {
				// Some failure paths (such as a rejected callback signature) return
				// before the handler reads the request body. Drain only up to the
				// detail capture limit plus one byte to keep the read bounded.
				_, _ = io.CopyN(io.Discard, diagnosticRequestBody, diagnosticDetailLimit+1)
			}
			if recovered := recover(); recovered != nil {
				message, truncated := limitDiagnosticString(fmt.Sprintf("panic: %v", recovered))
				entry := diagnosticEntryForRequest(r, requestHeaders, requestHeadersTruncated, requestBody, wrapped, "panic", http.StatusInternalServerError)
				entry.At = time.Now().UTC()
				entry.DurationMS = time.Since(started).Milliseconds()
				entry.ResponseHeaders, entry.ResponseHeadersTruncated = cloneDiagnosticHeaders(w.Header())
				entry.Message, entry.MessageTruncated = message, truncated
				s.appendDiagnostic(entry)
				panic(recovered)
			}

			status := wrapped.status
			if status == 0 {
				status = http.StatusOK
			}
			kind := "wechat_callback"
			if !isWechatCallbackPath(r.URL.Path) {
				if status < http.StatusBadRequest {
					return
				}
				kind = "http_error"
			}
			entry := diagnosticEntryForRequest(r, requestHeaders, requestHeadersTruncated, requestBody, wrapped, kind, status)
			entry.At = time.Now().UTC()
			entry.DurationMS = time.Since(started).Milliseconds()
			entry.ResponseHeaders, entry.ResponseHeadersTruncated = cloneDiagnosticHeaders(w.Header())
			s.appendDiagnostic(entry)
		}()

		next.ServeHTTP(wrapped, r)
	})
}

func diagnosticEntryForRequest(r *http.Request, requestHeaders http.Header, requestHeadersTruncated bool, requestBody *diagnosticBodyCapture, response *diagnosticResponseWriter, kind string, status int) requestDiagnosticEntry {
	requestBodyValue, requestBodyTruncated := requestBody.value()
	responseBodyValue, responseBodyTruncated := response.capture.value()
	urlValue, urlTruncated := limitDiagnosticString(r.URL.String())
	queryValue, queryTruncated := limitDiagnosticString(r.URL.RawQuery)
	return requestDiagnosticEntry{
		Kind:                    kind,
		Method:                  r.Method,
		Path:                    limitDiagnosticField(r.URL.Path),
		URL:                     urlValue,
		URLTruncated:            urlTruncated,
		Query:                   queryValue,
		QueryTruncated:          queryTruncated,
		Status:                  status,
		ProbeID:                 diagnosticProbeID(r),
		Host:                    limitDiagnosticField(r.Host),
		RemoteAddr:              limitDiagnosticField(r.RemoteAddr),
		Proto:                   limitDiagnosticField(forwardedProto(r)),
		Cloudflare:              limitDiagnosticField(r.Header.Get("CF-Ray")),
		UserAgent:               limitDiagnosticField(r.UserAgent()),
		RequestHeaders:          requestHeaders,
		RequestHeadersTruncated: requestHeadersTruncated,
		RequestBody:             requestBodyValue,
		RequestBodyTruncated:    requestBodyTruncated,
		ResponseBody:            responseBodyValue,
		ResponseBodyTruncated:   responseBodyTruncated,
	}
}

func diagnosticPath(path string) bool {
	switch path {
	case "/login/wechat", "/login/wechat-code", "/auth/wechat/callback", "/auth/wechat/code/login", "/auth/wechat/code/callback", "/healthz", "/__diag/ping":
		return true
	default:
		return false
	}
}

func isWechatCallbackPath(path string) bool {
	return path == WeChatCallbackPath || path == "/auth/wechat/code/callback"
}

type diagnosticBodyCapture struct {
	buffer    bytes.Buffer
	truncated bool
}

func (c *diagnosticBodyCapture) write(body []byte) {
	remaining := diagnosticDetailLimit - c.buffer.Len()
	if remaining <= 0 {
		if len(body) > 0 {
			c.truncated = true
		}
		return
	}
	if len(body) > remaining {
		_, _ = c.buffer.Write(body[:remaining])
		c.truncated = true
		return
	}
	_, _ = c.buffer.Write(body)
}

func (c *diagnosticBodyCapture) value() (string, bool) {
	return c.buffer.String(), c.truncated
}

type diagnosticReadCloser struct {
	io.ReadCloser
	capture *diagnosticBodyCapture
}

func (r *diagnosticReadCloser) Read(body []byte) (int, error) {
	n, err := r.ReadCloser.Read(body)
	if n > 0 {
		r.capture.write(body[:n])
	}
	return n, err
}

type diagnosticResponseWriter struct {
	http.ResponseWriter
	status  int
	capture *diagnosticBodyCapture
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
	w.capture.write(body)
	return w.ResponseWriter.Write(body)
}

func (w *diagnosticResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func cloneDiagnosticHeaders(headers http.Header) (http.Header, bool) {
	cloned := make(http.Header)
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	remaining := diagnosticDetailLimit
	truncated := false
	for _, key := range keys {
		for _, value := range headers[key] {
			cost := len(key) + len(value)
			if cost > remaining {
				if remaining > len(key) {
					cloned.Add(key, value[:remaining-len(key)])
				}
				return cloned, true
			}
			cloned.Add(key, value)
			remaining -= cost
		}
	}
	return cloned, truncated
}

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

func limitDiagnosticString(value string) (string, bool) {
	if len(value) <= diagnosticDetailLimit {
		return value, false
	}
	return value[:diagnosticDetailLimit], true
}

func limitDiagnosticField(value string) string {
	limited, _ := limitDiagnosticString(value)
	return limited
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
	message := fallback
	if err != nil {
		message = err.Error()
	}
	message, truncated := limitDiagnosticString(message)
	urlValue, urlTruncated := limitDiagnosticString(r.URL.String())
	queryValue, queryTruncated := limitDiagnosticString(r.URL.RawQuery)
	s.appendDiagnostic(requestDiagnosticEntry{
		At: time.Now().UTC(), Kind: "error", Method: r.Method, Path: limitDiagnosticField(r.URL.Path),
		URL: urlValue, URLTruncated: urlTruncated, Query: queryValue, QueryTruncated: queryTruncated,
		Status: http.StatusBadGateway, ProbeID: diagnosticProbeID(r),
		Host: limitDiagnosticField(r.Host), RemoteAddr: limitDiagnosticField(r.RemoteAddr), Proto: limitDiagnosticField(forwardedProto(r)),
		Cloudflare: limitDiagnosticField(r.Header.Get("CF-Ray")), UserAgent: limitDiagnosticField(r.UserAgent()),
		Message: message, MessageTruncated: truncated,
	})
}

func (s *Server) recordWechatExchange(ctx context.Context, trace wechat.HTTPExchangeTrace) {
	probeID, _ := ctx.Value(diagnosticProbeContextKey{}).(string)
	urlValue, urlTruncated := limitDiagnosticString(trace.URL)
	s.appendDiagnostic(requestDiagnosticEntry{
		At: trace.At, Kind: "wechat_api", Method: trace.Method, Path: "微信 API", URL: urlValue, URLTruncated: urlTruncated,
		Status: trace.Status, DurationMS: trace.DurationMS, ProbeID: probeID,
		RequestHeaders: trace.RequestHeaders, RequestHeadersTruncated: trace.RequestHeadersTruncated,
		RequestBody: trace.RequestBody, RequestBodyTruncated: trace.RequestBodyTruncated,
		ResponseHeaders: trace.ResponseHeaders, ResponseHeadersTruncated: trace.ResponseHeadersTruncated,
		ResponseBody: trace.ResponseBody, ResponseBodyTruncated: trace.ResponseBodyTruncated,
		Message: trace.Error, MessageTruncated: trace.ErrorTruncated,
	})
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
