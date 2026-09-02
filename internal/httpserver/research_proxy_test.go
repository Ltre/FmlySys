package httpserver

import (
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestResearchIPAllowed(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.169.254", "::1", "fc00::1"}
	for _, raw := range blocked {
		if researchIPAllowed(net.ParseIP(raw)) {
			t.Fatalf("expected blocked IP: %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !researchIPAllowed(net.ParseIP(raw)) {
			t.Fatalf("expected public IP: %s", raw)
		}
	}
}

func TestNormalizeResearchTarget(t *testing.T) {
	valid := []string{"example.com", "https://example.com/a?q=1", "http://8.8.8.8/"}
	for _, raw := range valid {
		if _, err := normalizeResearchTarget(raw); err != nil {
			t.Fatalf("valid target %q: %v", raw, err)
		}
	}
	invalid := []string{"file:///etc/passwd", "http://localhost/", "http://127.0.0.1/", "https://example.com:8443/", "https://user:pass@example.com/"}
	for _, raw := range invalid {
		if _, err := normalizeResearchTarget(raw); err == nil {
			t.Fatalf("expected invalid target: %s", raw)
		}
	}
}

func TestRewriteResearchHTML(t *testing.T) {
	base, _ := url.Parse("https://example.com/a/page.html")
	got := rewriteResearchHTML(7, base, `<html><script>alert(1)</script><a href="../b">B</a><img src='/i.png' onerror="alert(2)"></html>`)
	if strings.Contains(strings.ToLower(got), "<script") || strings.Contains(strings.ToLower(got), "onerror") {
		t.Fatalf("active content remained: %s", got)
	}
	if !strings.Contains(got, "member=7") || !strings.Contains(got, "https%3A%2F%2Fexample.com%2Fb") {
		t.Fatalf("link was not proxied: %s", got)
	}
}
