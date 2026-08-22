package httpserver

import (
	"net/http/httptest"
	"testing"
)

func TestPasskeyRequestRPHTTPSDomain(t *testing.T) {
	r := httptest.NewRequest("GET", "https://fmly.miku.us/login", nil)
	rpID, origin, err := passkeyRequestRP(r)
	if err != nil {
		t.Fatal(err)
	}
	if rpID != "fmly.miku.us" || origin != "https://fmly.miku.us" {
		t.Fatalf("rpID=%q origin=%q", rpID, origin)
	}
}

func TestPasskeyRequestRPProxyHTTPS(t *testing.T) {
	r := httptest.NewRequest("GET", "http://fmly.miku.us/login", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	rpID, origin, err := passkeyRequestRP(r)
	if err != nil {
		t.Fatal(err)
	}
	if rpID != "fmly.miku.us" || origin != "https://fmly.miku.us" {
		t.Fatalf("rpID=%q origin=%q", rpID, origin)
	}
}

func TestPasskeyRequestRPLocalhostHTTP(t *testing.T) {
	r := httptest.NewRequest("GET", "http://localhost:8080/login", nil)
	rpID, origin, err := passkeyRequestRP(r)
	if err != nil {
		t.Fatal(err)
	}
	if rpID != "localhost" || origin != "http://localhost:8080" {
		t.Fatalf("rpID=%q origin=%q", rpID, origin)
	}
}

func TestPasskeyRequestRPRejectsLANHTTP(t *testing.T) {
	r := httptest.NewRequest("GET", "http://10.0.0.27:8080/login", nil)
	if _, _, err := passkeyRequestRP(r); err == nil {
		t.Fatal("expected non-secure LAN HTTP to be rejected")
	}
}
