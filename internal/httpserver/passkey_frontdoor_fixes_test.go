package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRewritePasskeySuccessToHome(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		http.SetCookie(w, &http.Cookie{Name: "fmly_passkey_identity", Value: "token", Path: "/"})
		_, _ = w.Write([]byte(`{"ok":true,"redirect":"/passkey/account?created=1"}`))
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/passkey/create/finish", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	rewritePasskeySuccessToHome(next)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"redirect":"/"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if got := rec.Header().Get("Set-Cookie"); !strings.Contains(got, "fmly_passkey_identity=token") {
		t.Fatalf("Set-Cookie lost: %q", got)
	}
}

func TestRewritePasskeyErrorUnchanged(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"message":"denied"}`))
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/passkey/login/finish", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	rewritePasskeySuccessToHome(next)(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusUnauthorized)
	}
	if strings.Contains(rec.Body.String(), `"redirect":"/"`) {
		t.Fatalf("error response was rewritten: %s", rec.Body.String())
	}
}
