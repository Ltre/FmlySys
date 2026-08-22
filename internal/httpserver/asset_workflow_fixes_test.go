package httpserver

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseWorkflowRequestURLEncodedAmount(t *testing.T) {
	body := url.Values{"amount": {"123.45"}, "event_type": {"ASSET_IN"}}.Encode()
	r := httptest.NewRequest("POST", "/assets/self-events", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	if _, err := parseWorkflowRequest(w, r, false); err != nil {
		t.Fatal(err)
	}
	if got := r.FormValue("amount"); got != "123.45" {
		t.Fatalf("amount=%q", got)
	}
}

func TestParseWorkflowRequestMultipartAmount(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("amount", "321.00"); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("event_type", "ASSET_OUT"); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/assets/self-events", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	if _, err := parseWorkflowRequest(w, r, false); err != nil {
		t.Fatal(err)
	}
	if got := r.FormValue("amount"); got != "321.00" {
		t.Fatalf("amount=%q", got)
	}
}

func TestEvidenceInlineWorkflow(t *testing.T) {
	for _, name := range []string{"a.jpg", "a.MP4", "a.mp3", "a.pdf", "a.txt"} {
		if !evidenceInlineWorkflow(name) {
			t.Fatalf("expected inline preview for %s", name)
		}
	}
	for _, name := range []string{"a.docx", "a.xlsx", "a.zip"} {
		if evidenceInlineWorkflow(name) {
			t.Fatalf("expected attachment download for %s", name)
		}
	}
}
