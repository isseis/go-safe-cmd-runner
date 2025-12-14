//go:build ignore

package main

import (
	"testing"
)

// TestIsPrivateIP tests the private IP detection function
func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected bool
		desc     string
	}{
		// RFC 1918 private addresses
		{"Private 10.x", "10.0.0.1", true, "10.0.0.0/8 range"},
		{"Private 172.16.x", "172.16.0.1", true, "172.16.0.0/12 range"},
		{"Private 192.168.x", "192.168.1.1", true, "192.168.0.0/16 range"},

		// Loopback addresses
		{"Loopback IPv4", "127.0.0.1", true, "IPv4 loopback"},
		{"Loopback localhost", "localhost", true, "localhost hostname"},
		{"Loopback IPv6", "::1", true, "IPv6 loopback"},

		// Link-local addresses
		{"Link-local 169.254.x", "169.254.169.254", true, "AWS metadata endpoint"},
		{"Link-local IPv6", "fe80::1", true, "IPv6 link-local"},

		// Public addresses (should NOT be private)
		{"Public Google DNS", "8.8.8.8", false, "Public DNS server"},
		{"Public Cloudflare", "1.1.1.1", false, "Public DNS server"},

		// Edge cases
		{"Empty string", "", true, "Empty should be blocked (conservative)"},
		{"Invalid IP", "not-an-ip", true, "Invalid should be blocked (conservative)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrivateIP(tt.host)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%q) = %v; want %v (%s)",
					tt.host, result, tt.expected, tt.desc)
			}
		})
	}
}

// TestIsAllowedHost tests the URL allowlist validation
func TestIsAllowedHost(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantError bool
		desc      string
	}{
		// Allowed hosts
		{"Allowed github.com", "https://github.com/user/repo", false, "GitHub is in allowlist"},
		{"Allowed golang.org", "https://golang.org/doc/", false, "golang.org is in allowlist"},
		{"Allowed pkg.go.dev", "https://pkg.go.dev/net/http", false, "pkg.go.dev is in allowlist"},
		{"Allowed wikipedia", "https://en.wikipedia.org/wiki/SSRF", false, "Wikipedia is in allowlist"},

		// Not in allowlist
		{"Not allowed example.com", "https://example.com/page", true, "example.com not in allowlist"},
		{"Not allowed random.org", "http://random.org/", true, "random.org not in allowlist"},

		// Private IPs (SSRF attack vectors)
		{"SSRF localhost", "http://localhost:8080/admin", true, "Localhost should be blocked"},
		{"SSRF 127.0.0.1", "http://127.0.0.1:9000/", true, "Loopback IP should be blocked"},
		{"SSRF 10.x private", "http://10.0.0.1/secret", true, "RFC 1918 10.x should be blocked"},
		{"SSRF 192.168.x", "http://192.168.1.1/admin", true, "RFC 1918 192.168.x should be blocked"},
		{"SSRF 172.16.x", "http://172.16.0.1/internal", true, "RFC 1918 172.16.x should be blocked"},
		{"SSRF AWS metadata", "http://169.254.169.254/latest/meta-data/", true, "AWS metadata endpoint should be blocked"},

		// IPv6 private addresses
		{"SSRF IPv6 loopback", "http://[::1]:8080/", true, "IPv6 loopback should be blocked"},
		{"SSRF IPv6 link-local", "http://[fe80::1]/", true, "IPv6 link-local should be blocked"},

		// Invalid URLs
		{"Invalid URL", "not a url", true, "Invalid URL should be rejected"},
		{"Empty URL", "", true, "Empty URL should be rejected"},

		// Edge cases with ports
		{"Allowed with port", "https://github.com:443/repo", true, "Port in Host should not match allowlist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := isAllowedHost(tt.url)
			if (err != nil) != tt.wantError {
				t.Errorf("isAllowedHost(%q) error = %v; wantError = %v (%s)",
					tt.url, err, tt.wantError, tt.desc)
			}
		})
	}
}

// TestSSRFAttackVectors tests common SSRF attack patterns
func TestSSRFAttackVectors(t *testing.T) {
	// Common SSRF attack vectors that MUST be blocked
	attackVectors := []struct {
		name string
		url  string
		desc string
	}{
		{
			name: "AWS metadata v1",
			url:  "http://169.254.169.254/latest/meta-data/",
			desc: "AWS EC2 instance metadata service",
		},
		{
			name: "AWS metadata v2",
			url:  "http://169.254.169.254/latest/api/token",
			desc: "AWS EC2 IMDSv2 token endpoint",
		},
		{
			name: "GCP metadata",
			url:  "http://metadata.google.internal/computeMetadata/v1/",
			desc: "Google Cloud Platform metadata",
		},
		{
			name: "Azure metadata",
			url:  "http://169.254.169.254/metadata/instance?api-version=2021-02-01",
			desc: "Azure instance metadata service",
		},
		{
			name: "Local admin panel",
			url:  "http://localhost:8080/admin",
			desc: "Local web admin interface",
		},
		{
			name: "Internal API",
			url:  "http://127.0.0.1:3000/api/secrets",
			desc: "Local API endpoint",
		},
		{
			name: "Docker daemon",
			url:  "http://127.0.0.1:2375/containers/json",
			desc: "Docker daemon API",
		},
		{
			name: "Kubernetes API",
			url:  "http://127.0.0.1:8001/api/v1/namespaces",
			desc: "Kubernetes API proxy",
		},
		{
			name: "Private network web",
			url:  "http://192.168.1.100/admin",
			desc: "Private network web interface",
		},
		{
			name: "Internal database",
			url:  "http://10.0.0.5:5432/",
			desc: "Internal database port scan",
		},
	}

	for _, av := range attackVectors {
		t.Run(av.name, func(t *testing.T) {
			err := isAllowedHost(av.url)
			if err == nil {
				t.Errorf("SECURITY FAILURE: %s was not blocked!\nURL: %s\nDescription: %s",
					av.name, av.url, av.desc)
			} else {
				t.Logf("✓ Correctly blocked %s: %v", av.name, err)
			}
		})
	}
}

// TestAllowlistBypass tests that attackers cannot bypass the allowlist
func TestAllowlistBypass(t *testing.T) {
	bypassAttempts := []struct {
		name string
		url  string
		desc string
	}{
		{
			name: "Port addition",
			url:  "https://github.com:443/user/repo",
			desc: "Trying to bypass by adding default HTTPS port",
		},
		{
			name: "Custom port",
			url:  "https://github.com:8080/",
			desc: "Trying to bypass with non-standard port",
		},
		{
			name: "Subdomain",
			url:  "https://api.github.com/repos",
			desc: "Subdomain not explicitly in allowlist",
		},
		{
			name: "Different subdomain",
			url:  "https://evil.github.com/",
			desc: "Different subdomain (evil.github.com) not in allowlist",
		},
	}

	for _, ba := range bypassAttempts {
		t.Run(ba.name, func(t *testing.T) {
			err := isAllowedHost(ba.url)
			// These should all be blocked because the Host (with port) or subdomain
			// is not exactly in the allowlist
			if err == nil {
				t.Errorf("SECURITY FAILURE: Bypass attempt succeeded!\nURL: %s\nDescription: %s",
					ba.url, ba.desc)
			} else {
				t.Logf("✓ Correctly blocked bypass attempt: %v", err)
			}
		})
	}
}

// TestLegitimateURLs ensures legitimate URLs are still allowed
func TestLegitimateURLs(t *testing.T) {
	legitimateURLs := []struct {
		name string
		url  string
		desc string
	}{
		{
			name: "GitHub repository",
			url:  "https://github.com/golang/go",
			desc: "Public GitHub repository",
		},
		{
			name: "Go documentation",
			url:  "https://pkg.go.dev/net/http",
			desc: "Go package documentation",
		},
		{
			name: "Golang.org docs",
			url:  "https://golang.org/doc/effective_go",
			desc: "Official Go documentation",
		},
		{
			name: "Wikipedia article",
			url:  "https://en.wikipedia.org/wiki/SQL_injection",
			desc: "Wikipedia reference",
		},
		{
			name: "OWASP reference",
			url:  "https://owasp.org/www-community/attacks/Server_Side_Request_Forgery",
			desc: "OWASP security reference",
		},
	}

	for _, lu := range legitimateURLs {
		t.Run(lu.name, func(t *testing.T) {
			err := isAllowedHost(lu.url)
			if err != nil {
				t.Errorf("FUNCTIONALITY FAILURE: Legitimate URL was blocked!\nURL: %s\nDescription: %s\nError: %v",
					lu.url, lu.desc, err)
			} else {
				t.Logf("✓ Correctly allowed: %s", lu.url)
			}
		})
	}
}
