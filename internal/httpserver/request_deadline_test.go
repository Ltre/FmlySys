package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWithRequestDeadlineAddsFiniteDeadline(t *testing.T) {
	var hasDeadline bool
	h := WithRequestDeadline(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasDeadline = r.Context().Deadline()
		w.WriteHeader(http.StatusNoContent)
	}), time.Second)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !hasDeadline {
		t.Fatal("expected request context deadline")
	}
}
