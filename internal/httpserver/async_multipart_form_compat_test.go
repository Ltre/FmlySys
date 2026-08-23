package httpserver

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAsyncMultipartCompatibilityNormalizesFieldOnlyForm(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("member_id", "1")
	_ = writer.WriteField("name", "张三")
	_ = writer.WriteField("permissions", "assets.view")
	_ = writer.WriteField("permissions", "share.view")
	_ = writer.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
			t.Fatalf("Content-Type=%q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("member_id"); got != "1" {
			t.Fatalf("member_id=%q", got)
		}
		if got := r.FormValue("name"); got != "张三" {
			t.Fatalf("name=%q", got)
		}
		if got := r.Form["permissions"]; len(got) != 2 || got[0] != "assets.view" || got[1] != "share.view" {
			t.Fatalf("permissions=%v", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/members", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(enhancedFormHeader, "1")
	rec := httptest.NewRecorder()
	WithAsyncMultipartFormCompatibility(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestAsyncMultipartCompatibilityKeepsFileUploadMultipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("title", "凭证")
	part, err := writer.CreateFormFile("evidence", "proof.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("ok"))
	_ = writer.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data") {
			t.Fatalf("Content-Type=%q", got)
		}
		if err := r.ParseMultipartForm(asyncMultipartCompatMaxBytes); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("title"); got != "凭证" {
			t.Fatalf("title=%q", got)
		}
		if files := r.MultipartForm.File["evidence"]; len(files) != 1 || files[0].Filename != "proof.txt" {
			t.Fatalf("files=%v", files)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/assets/expenses", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(enhancedFormHeader, "1")
	rec := httptest.NewRecorder()
	WithAsyncMultipartFormCompatibility(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rec.Code)
	}
}
