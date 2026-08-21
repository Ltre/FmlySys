package httpserver

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// WithRequestDeadline gives normal database-backed page/auth requests a finite
// context deadline. Large multipart uploads and file downloads are excluded so
// transfer duration is not accidentally capped by the database safety timeout.
func WithRequestDeadline(next http.Handler, timeout time.Duration) http.Handler {
	if timeout <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/") ||
			strings.HasPrefix(r.URL.Path, "/files/") || strings.HasPrefix(r.URL.Path, "/evidence/") {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
