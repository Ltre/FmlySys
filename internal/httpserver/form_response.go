package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

const enhancedFormHeader = "X-Fmly-Async"

type enhancedFormResponse struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
	Redirect string `json:"redirect,omitempty"`
}

type captureResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newCaptureResponseWriter() *captureResponseWriter {
	return &captureResponseWriter{header: make(http.Header), status: http.StatusOK}
}

func (w *captureResponseWriter) Header() http.Header { return w.header }

func (w *captureResponseWriter) WriteHeader(status int) {
	if w.status != http.StatusOK || w.body.Len() > 0 {
		return
	}
	w.status = status
}

func (w *captureResponseWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

// WithEnhancedFormResponses keeps the existing POST handlers and validation as
// the source of truth while translating their redirect/error responses into
// JSON only for progressive-enhancement requests from app.js.
func WithEnhancedFormResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(enhancedFormHeader) != "1" {
			next.ServeHTTP(w, r)
			return
		}

		capture := newCaptureResponseWriter()
		next.ServeHTTP(capture, r)

		copyEnhancedHeaders(w.Header(), capture.header)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		if capture.status >= 300 && capture.status < 400 {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(enhancedFormResponse{
				OK:       true,
				Redirect: capture.header.Get("Location"),
			})
			return
		}

		if capture.status >= 400 {
			message := strings.TrimSpace(capture.body.String())
			if message == "" {
				message = http.StatusText(capture.status)
			}
			w.WriteHeader(capture.status)
			_ = json.NewEncoder(w).Encode(enhancedFormResponse{
				OK:      false,
				Message: message,
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(enhancedFormResponse{OK: true})
	})
}

func copyEnhancedHeaders(dst, src http.Header) {
	for key, values := range src {
		switch strings.ToLower(key) {
		case "content-type", "content-length", "location":
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
