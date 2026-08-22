package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnhancedFormResponseConvertsRedirect(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets", http.StatusSeeOther)
	})
	req := httptest.NewRequest(http.MethodPost, "/assets/reimbursements", nil)
	req.Header.Set(enhancedFormHeader, "1")
	rec := httptest.NewRecorder()

	WithEnhancedFormResponses(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	var got enhancedFormResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Redirect != "/assets" {
		t.Fatalf("response=%+v", got)
	}
}

func TestEnhancedFormResponseKeepsValidationOnPage(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "报销付款人的公共资产虚拟账户余额不足", http.StatusBadRequest)
	})
	req := httptest.NewRequest(http.MethodPost, "/assets/reimbursements", nil)
	req.Header.Set(enhancedFormHeader, "1")
	rec := httptest.NewRecorder()

	WithEnhancedFormResponses(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
	var got enhancedFormResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OK || got.Message != "报销付款人的公共资产虚拟账户余额不足" {
		t.Fatalf("response=%+v", got)
	}
}

func TestEnhancedFormResponseLeavesNormalPostUntouched(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets", http.StatusSeeOther)
	})
	req := httptest.NewRequest(http.MethodPost, "/assets/reimbursements", nil)
	rec := httptest.NewRecorder()

	WithEnhancedFormResponses(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d, want 303", rec.Code)
	}
	if rec.Header().Get("Location") != "/assets" {
		t.Fatalf("Location=%q", rec.Header().Get("Location"))
	}
}
