//go:build ignore

// verify_links.go - Verify internal and external links in documentation
//
// This script checks for broken links in markdown documentation files.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
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
			allLinks[i].IsValid = verifyInternalLink(allLinks[i].URL, config.DocsRoot)
			if !allLinks[i].IsValid {
				allLinks[i].Error = "File or anchor not found"
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
func verifyInternalLink(url, docsRoot string) bool {
	// Remove anchor if present
	parts := strings.SplitN(url, "#", 2)
	filePath := parts[0]

	// Skip empty paths (anchor-only links)
	if filePath == "" {
		return true
	}

	// Convert relative path to absolute
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(docsRoot, filePath)
	}

	// Check if file exists
	_, err := os.Stat(filePath)
	return err == nil
}

// verifyExternalLink verifies an external link
func verifyExternalLink(url string, timeoutSec int) (bool, error) {
	client := &http.Client{
		Timeout: time.Duration(timeoutSec) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
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
				fmt.Printf("    File: %s:%d\n", filepath.Base(link.SourceFile), link.Line)
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
				fmt.Printf("    File: %s:%d\n", filepath.Base(link.SourceFile), link.Line)
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
