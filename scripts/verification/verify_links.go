//go:build ignore

// verify_links.go - Verify internal and external links in documentation
//
// This script checks for broken links in markdown documentation files.
//
// SECURITY WARNING:
// External link checking makes HTTP requests to URLs found in documentation.
// To prevent Server-Side Request Forgery (SSRF) attacks:
// - Only trusted domains are allowed (see allowedHosts)
// - Private IP addresses are blocked (RFC 1918, loopback, link-local)
// - DNS rebinding attacks are mitigated
// See docs/security/SSRF-001-external-link-verification.md for details.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Link represents a link found in documentation
type Link struct {
	Type       string `json:"type"` // "internal" or "external"
	URL        string `json:"url"`
	Text       string `json:"text"`
	SourceFile string `json:"source_file"`
	Line       int    `json:"line"`
	IsValid    bool   `json:"is_valid"`
	Error      string `json:"error,omitempty"`
}

// Config holds the script configuration
type Config struct {
	DocsRoot      string
	CheckExternal bool
	OutputJSON    string
	Verbose       bool
	Timeout       int
}

// allowedHosts is a map of trusted domains for external link checking.
// SECURITY: Only these hosts can be checked to prevent SSRF attacks.
// Add new trusted domains here as needed.
var allowedHosts = map[string]bool{
	// Go ecosystem
	"golang.org":          true,
	"go.dev":              true,
	"pkg.go.dev":          true,
	"go.googlesource.com": true,

	// Documentation and references
	"en.wikipedia.org": true,
	"ja.wikipedia.org": true,
	"owasp.org":        true,
	"cwe.mitre.org":    true,

	// Code hosting
	"github.com": true,
	"gitlab.com": true,

	// Rust ecosystem (if needed)
	"docs.rs":   true,
	"crates.io": true,

	// Standards and RFCs
	"www.rfc-editor.org":   true,
	"datatracker.ietf.org": true,
}

func main() {
	config := parseFlags()

	// Find all documentation files
	files, err := findMarkdownFiles(config.DocsRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding files: %v\n", err)
		os.Exit(1)
	}

	// Extract and verify links
	allLinks := []Link{}
	for _, file := range files {
		links, err := extractLinks(file, config.DocsRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting links from %s: %v\n", file, err)
			continue
		}
		allLinks = append(allLinks, links...)
	}

	// Verify internal links
	for i := range allLinks {
		if allLinks[i].Type == "internal" {
			allLinks[i].IsValid = verifyInternalLink(allLinks[i].URL, allLinks[i].SourceFile)
			if !allLinks[i].IsValid {
				// Debug: print resolved path
				dir := filepath.Dir(allLinks[i].SourceFile)
				resolved := filepath.Join(dir, allLinks[i].URL)
				// Remove anchor for path check
				if idx := strings.Index(resolved, "#"); idx != -1 {
					resolved = resolved[:idx]
				}
				allLinks[i].Error = fmt.Sprintf("File or anchor not found (Resolved: %s)", resolved)
			}
		}
	}

	// Verify external links if requested
	if config.CheckExternal {
		for i := range allLinks {
			if allLinks[i].Type == "external" {
				isValid, err := verifyExternalLink(allLinks[i].URL, config.Timeout)
				allLinks[i].IsValid = isValid
				if err != nil {
					allLinks[i].Error = err.Error()
				}
			}
		}
	} else {
		// Mark all external links as valid if not checking
		for i := range allLinks {
			if allLinks[i].Type == "external" {
				allLinks[i].IsValid = true
			}
		}
	}

	// Print report
	printReport(allLinks, config.Verbose)

	// Write JSON if requested
	if config.OutputJSON != "" {
		if err := writeJSONReport(config.OutputJSON, allLinks); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON report: %v\n", err)
			os.Exit(1)
		}
	}

	// Exit with error if broken links found
	brokenCount := 0
	for _, link := range allLinks {
		if !link.IsValid {
			brokenCount++
		}
	}

	if brokenCount > 0 {
		os.Exit(1)
	}
}

func parseFlags() *Config {
	config := &Config{}
	flag.StringVar(&config.DocsRoot, "docs", "docs", "Root directory of documentation")
	flag.BoolVar(&config.CheckExternal, "external", false, "Check external links (may be slow)")
	flag.StringVar(&config.OutputJSON, "output", "", "Output JSON file path (optional)")
	flag.BoolVar(&config.Verbose, "verbose", false, "Verbose output")
	flag.IntVar(&config.Timeout, "timeout", 10, "Timeout for external link checks (seconds)")
	flag.Parse()
	return config
}

// findMarkdownFiles finds all markdown files
func findMarkdownFiles(rootDir string) ([]string, error) {
	var files []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".ja.md")) {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// extractLinks extracts all links from a markdown file
func extractLinks(filePath, docsRoot string) ([]Link, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	links := []Link{}

	// Regex patterns for different link types
	// Markdown: [text](url)
	markdownLinkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^\)]+)\)`)
	// Reference: [text]: url
	referenceLinkRegex := regexp.MustCompile(`^\[([^\]]+)\]:\s*(.+)$`)

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Extract markdown-style links
		matches := markdownLinkRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				text := match[1]
				url := match[2]

				link := Link{
					Text:       text,
					URL:        url,
					SourceFile: filePath,
					Line:       lineNum,
				}

				if isExternalURL(url) {
					link.Type = "external"
				} else {
					link.Type = "internal"
				}

				links = append(links, link)
			}
		}

		// Extract reference-style links
		matches = referenceLinkRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				text := match[1]
				url := match[2]

				link := Link{
					Text:       text,
					URL:        url,
					SourceFile: filePath,
					Line:       lineNum,
				}

				if isExternalURL(url) {
					link.Type = "external"
				} else {
					link.Type = "internal"
				}

				links = append(links, link)
			}
		}
	}

	return links, scanner.Err()
}

// isExternalURL checks if a URL is external
func isExternalURL(url string) bool {
	return strings.HasPrefix(url, "http://") ||
		strings.HasPrefix(url, "https://") ||
		strings.HasPrefix(url, "ftp://")
}

// verifyInternalLink verifies an internal link
func verifyInternalLink(url, sourceFile string) bool {
	// Remove anchor if present
	parts := strings.SplitN(url, "#", 2)
	filePath := parts[0]

	// Skip empty paths (anchor-only links)
	if filePath == "" {
		return true
	}

	// Convert relative path to absolute
	if !filepath.IsAbs(filePath) {
		sourceDir := filepath.Dir(sourceFile)
		filePath = filepath.Join(sourceDir, filePath)
	}

	// Check if file exists
	_, err := os.Stat(filePath)
	return err == nil
}

// isPrivateIP checks if an IP address is in a private range.
// SECURITY: Blocks access to internal networks to prevent SSRF.
func isPrivateIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		// Try resolving hostname to IP
		addrs, err := net.LookupIP(host)
		if err != nil || len(addrs) == 0 {
			// Cannot resolve - be conservative and block
			return true
		}
		// SECURITY: Check ALL returned IP addresses, not just the first.
		// A malicious DNS server could return a public IP first and private IPs later
		// to bypass this check.
		for _, addr := range addrs {
			if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() {
				return true
			}
		}
		return false
	}

	// Check for private, loopback, and link-local addresses
	// These include:
	// - 10.0.0.0/8 (RFC 1918)
	// - 172.16.0.0/12 (RFC 1918)
	// - 192.168.0.0/16 (RFC 1918)
	// - 127.0.0.0/8 (loopback)
	// - 169.254.0.0/16 (link-local)
	// - fc00::/7 (IPv6 unique local)
	// - fe80::/10 (IPv6 link-local)
	// - ::1 (IPv6 loopback)
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// isAllowedHost checks if a URL's host is in the allowlist.
// SECURITY: Only allowlisted hosts can be accessed to prevent SSRF.
func isAllowedHost(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Extract hostname without port
	hostname := u.Hostname()

	// Block all private/internal IP ranges
	if isPrivateIP(hostname) {
		return fmt.Errorf("private IP addresses are not allowed: %s", hostname)
	}

	// Check against allowlist
	if !allowedHosts[u.Host] {
		return fmt.Errorf("host not in allowlist: %s (add to allowedHosts if trusted)", u.Host)
	}

	return nil
}

// verifyExternalLink verifies an external link with SSRF protection.
// SECURITY: Validates URL against allowlist and blocks private IPs.
func verifyExternalLink(url string, timeoutSec int) (bool, error) {
	// SECURITY: Validate URL before making any requests
	if err := isAllowedHost(url); err != nil {
		return false, fmt.Errorf("security check failed: %w", err)
	}
	// SECURITY: Custom transport with DNS rebinding protection
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// SECURITY: Re-validate IP addresses at connection time to prevent DNS rebinding
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, fmt.Errorf("invalid address: %w", err)
				}
				if isPrivateIP(host) {
					return nil, fmt.Errorf("connection blocked to private IP: %s", host)
				}
				return (&net.Dialer{
					Timeout:   time.Duration(timeoutSec) * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext(ctx, network, addr)
			},
		},
		Timeout: time.Duration(timeoutSec) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// SECURITY: Validate redirect targets against allowlist
			if err := isAllowedHost(req.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			// Allow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Use HEAD request for efficiency
	resp, err := client.Head(url)
	if err != nil {
		// Try GET if HEAD fails
		resp, err = client.Get(url)
		if err != nil {
			return false, err
		}
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx as success
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true, nil
	}

	return false, fmt.Errorf("HTTP %d", resp.StatusCode)
}

// printReport prints the verification report
func printReport(links []Link, verbose bool) {
	fmt.Printf("=== Link Verification Report ===\n\n")

	internalCount := 0
	externalCount := 0
	brokenInternal := 0
	brokenExternal := 0

	for _, link := range links {
		if link.Type == "internal" {
			internalCount++
			if !link.IsValid {
				brokenInternal++
			}
		} else {
			externalCount++
			if !link.IsValid {
				brokenExternal++
			}
		}
	}

	fmt.Printf("Total links: %d\n", len(links))
	fmt.Printf("  Internal: %d (broken: %d)\n", internalCount, brokenInternal)
	fmt.Printf("  External: %d (broken: %d)\n", externalCount, brokenExternal)
	fmt.Printf("\n")

	// Report broken internal links
	if brokenInternal > 0 {
		fmt.Printf("⚠️  Broken Internal Links (%d):\n", brokenInternal)
		for _, link := range links {
			if link.Type == "internal" && !link.IsValid {
				fmt.Printf("  - %s\n", link.URL)
				relPath := link.SourceFile
				if cwd, err := os.Getwd(); err == nil {
					if rel, err := filepath.Rel(cwd, link.SourceFile); err == nil {
						relPath = rel
					}
				}
				fmt.Printf("    File: %s:%d\n", relPath, link.Line)
				if link.Error != "" {
					fmt.Printf("    Error: %s\n", link.Error)
				}
			}
		}
		fmt.Printf("\n")
	}

	// Report broken external links
	if brokenExternal > 0 {
		fmt.Printf("⚠️  Broken External Links (%d):\n", brokenExternal)
		for _, link := range links {
			if link.Type == "external" && !link.IsValid {
				fmt.Printf("  - %s\n", link.URL)
				relPath := link.SourceFile
				if cwd, err := os.Getwd(); err == nil {
					if rel, err := filepath.Rel(cwd, link.SourceFile); err == nil {
						relPath = rel
					}
				}
				fmt.Printf("    File: %s:%d\n", relPath, link.Line)
				if link.Error != "" {
					fmt.Printf("    Error: %s\n", link.Error)
				}
			}
		}
		fmt.Printf("\n")
	}

	// Verbose: show all links
	if verbose {
		fmt.Printf("All Links:\n")
		currentFile := ""
		for _, link := range links {
			if link.SourceFile != currentFile {
				currentFile = link.SourceFile
				fmt.Printf("\n📄 %s\n", filepath.Base(currentFile))
			}

			status := "✅"
			if !link.IsValid {
				status = "❌"
			}

			fmt.Printf("  %s [%s] %s\n", status, link.Type, link.URL)
			if link.Error != "" {
				fmt.Printf("      Error: %s\n", link.Error)
			}
		}
		fmt.Printf("\n")
	}

	if brokenInternal == 0 && brokenExternal == 0 {
		fmt.Printf("✅ All links are valid!\n")
	}
}

// writeJSONReport writes the report as JSON
func writeJSONReport(path string, links []Link) error {
	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
