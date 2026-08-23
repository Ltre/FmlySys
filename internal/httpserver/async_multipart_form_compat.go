package httpserver

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

const asyncMultipartCompatMaxBytes = 1 << 20

// WithAsyncMultipartFormCompatibility normalizes small field-only multipart
// requests emitted by older cached versions of app.js. Those versions wrapped
// every async POST in FormData, while many legacy handlers intentionally use
// ParseForm and therefore ignore multipart fields. File uploads remain
// multipart and keep their existing upload-specific handling and limits.
func WithAsyncMultipartFormCompatibility(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(enhancedFormHeader) != "1" ||
			!strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") ||
			r.ContentLength <= 0 || r.ContentLength > asyncMultipartCompatMaxBytes {
			next.ServeHTTP(w, r)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, asyncMultipartCompatMaxBytes)
		if err := r.ParseMultipartForm(asyncMultipartCompatMaxBytes); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		multipartForm := r.MultipartForm
		if multipartForm != nil {
			defer multipartForm.RemoveAll()
		}
		if multipartForm == nil || multipartFormHasFiles(multipartForm.File) {
			next.ServeHTTP(w, r)
			return
		}

		values := make(url.Values, len(multipartForm.Value))
		for key, items := range multipartForm.Value {
			values[key] = append([]string(nil), items...)
		}
		encoded := values.Encode()
		r.Body = io.NopCloser(bytes.NewBufferString(encoded))
		r.ContentLength = int64(len(encoded))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Del("Content-Length")
		r.Form = nil
		r.PostForm = nil
		r.MultipartForm = nil

		next.ServeHTTP(w, r)
	})
}

func multipartFormHasFiles(files map[string][]*multipart.FileHeader) bool {
	for _, items := range files {
		if len(items) > 0 {
			return true
		}
	}
	return false
}
