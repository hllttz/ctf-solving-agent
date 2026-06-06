package tools

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestIsBlockedRejectsDirectPrivateAddresses(t *testing.T) {
	tests := []string{
		"127.0.0.1",
		"10.1.2.3",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.1.1",
		"169.254.1.1",
		"::1",
		"fc00::1",
		"fe80::1",
	}

	for _, host := range tests {
		t.Run(host, func(t *testing.T) {
			if !isBlocked(host) {
				t.Fatalf("isBlocked(%q) = false, want true", host)
			}
		})
	}
}

func TestIsBlockedAllowsDirectPublicAddresses(t *testing.T) {
	tests := []string{
		"8.8.8.8",
		"1.1.1.1",
		"2606:4700:4700::1111",
	}

	for _, host := range tests {
		t.Run(host, func(t *testing.T) {
			if isBlocked(host) {
				t.Fatalf("isBlocked(%q) = true, want false", host)
			}
		})
	}
}

func TestHasBlockedIPRejectsAnyPrivateAddress(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("8.8.8.8"),
		net.ParseIP("10.0.0.5"),
	}
	if !hasBlockedIP(ips) {
		t.Fatal("hasBlockedIP returned false with one private address")
	}
}

func TestCheckWebFetchRedirectBlocksPrivateTarget(t *testing.T) {
	req := &http.Request{URL: mustParseURL(t, "http://127.0.0.1/admin")}
	err := checkWebFetchRedirect(req, []*http.Request{
		{URL: mustParseURL(t, "http://example.com/start")},
	})
	if err == nil {
		t.Fatal("expected private redirect target to be blocked")
	}
	if !strings.Contains(err.Error(), "blocked redirect") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckWebFetchRedirectLimitsRedirects(t *testing.T) {
	req := &http.Request{URL: mustParseURL(t, "http://example.com/next")}
	via := []*http.Request{
		{URL: mustParseURL(t, "http://example.com/1")},
		{URL: mustParseURL(t, "http://example.com/2")},
		{URL: mustParseURL(t, "http://example.com/3")},
		{URL: mustParseURL(t, "http://example.com/4")},
		{URL: mustParseURL(t, "http://example.com/5")},
	}
	err := checkWebFetchRedirect(req, via)
	if err == nil {
		t.Fatal("expected redirect limit error")
	}
	if !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return u
}
