//go:build ignore

// verify_toml_keys.go - Extract and verify TOML configuration keys
//
// This script extracts TOML configuration keys from Go source code
// and compares them with keys documented in Japanese documentation files.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// TOMLKey represents a TOML configuration key found in code or documentation
type TOMLKey struct {
	Key          string   `json:"key"`
	Type         string   `json:"type,omitempty"`
	StructTag    string   `json:"struct_tag,omitempty"`
	SourceFile   string   `json:"source_file"`
	LineNumber   int      `json:"line_number"`
	ParentStruct string   `json:"parent_struct,omitempty"`
	DocFiles     []string `json:"doc_files,omitempty"`
}

// Config holds the script configuration
type Config struct {
	SourceRoot string
	DocsRoot   string
	OutputJSON string
	Verbose    bool
}

func main() {
	config := parseFlags()

	// Extract TOML keys from Go source code
	codeKeys, err := extractKeysFromCode(config.SourceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting keys from code: %v\n", err)
		os.Exit(1)
	}

	// Extract TOML keys from documentation
	docKeys, err := extractKeysFromDocs(config.DocsRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting keys from docs: %v\n", err)
		os.Exit(1)
	}

	// Compare and report
	report := compareKeys(codeKeys, docKeys)

	if config.OutputJSON != "" {
		if err := writeJSONReport(config.OutputJSON, report); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON report: %v\n", err)
			os.Exit(1)
		}
	}

	printReport(report, config.Verbose)
}

func parseFlags() *Config {
	config := &Config{}
	flag.StringVar(&config.SourceRoot, "source", ".", "Root directory of Go source code")
	flag.StringVar(&config.DocsRoot, "docs", "docs/user", "Root directory of documentation")
	flag.StringVar(&config.OutputJSON, "output", "", "Output JSON file path (optional)")
	flag.BoolVar(&config.Verbose, "verbose", false, "Verbose output")
	flag.Parse()
	return config
}

// extractKeysFromCode extracts TOML keys from Go struct tags
func extractKeysFromCode(rootDir string) (map[string]*TOMLKey, error) {
	keys := make(map[string]*TOMLKey)
	fset := token.NewFileSet()

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			typeSpec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return true
			}

			structName := typeSpec.Name.Name
			for _, field := range structType.Fields.List {
				if field.Tag == nil {
					continue
				}

				tag := field.Tag.Value
				tomlKey := extractTOMLKeyFromTag(tag)
				if tomlKey == "" {
					continue
				}

				var fieldType string
				if field.Type != nil {
					fieldType = fmt.Sprintf("%v", field.Type)
				}

				position := fset.Position(field.Pos())
				key := &TOMLKey{
					Key:          tomlKey,
					Type:         fieldType,
					StructTag:    strings.Trim(tag, "`"),
					SourceFile:   path,
					LineNumber:   position.Line,
					ParentStruct: structName,
				}

				keys[tomlKey] = key
			}

			return true
		})

		return nil
	})

	return keys, err
}

// extractTOMLKeyFromTag extracts TOML key from struct tag
func extractTOMLKeyFromTag(tag string) string {
	tag = strings.Trim(tag, "`")
	re := regexp.MustCompile(`toml:"([^"]+)"`)
	matches := re.FindStringSubmatch(tag)
	if len(matches) > 1 {
		// Extract the key name (before comma if present)
		keyParts := strings.Split(matches[1], ",")
		return keyParts[0]
	}
	return ""
}

// extractKeysFromDocs extracts TOML keys mentioned in documentation
func extractKeysFromDocs(docsRoot string) (map[string][]string, error) {
	keys := make(map[string][]string)

	// Patterns to match TOML keys in documentation
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`^\s*([a-z_]+)\s*=`),       // key = value
		regexp.MustCompile(`^\s*\[([a-z_\.]+)\]`),     // [section]
		regexp.MustCompile(`^\s*\[\[([a-z_\.]+)\]\]`), // [[array]]
		regexp.MustCompile("`([a-z_]+)`"),             // `key` in text
		regexp.MustCompile(`\*\*([a-z_]+)\*\*`),       // **key** in text
	}

	err := filepath.Walk(docsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".ja.md") {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		inCodeBlock := false
		codeBlockType := ""

		for scanner.Scan() {
			line := scanner.Text()

			// Track code block state
			if strings.HasPrefix(line, "```") {
				if !inCodeBlock {
					inCodeBlock = true
					codeBlockType = strings.TrimPrefix(line, "```")
				} else {
					inCodeBlock = false
					codeBlockType = ""
				}
				continue
			}

			// Only process TOML code blocks and regular text
			if inCodeBlock && codeBlockType != "toml" {
				continue
			}

			// Extract keys using all patterns
			for _, pattern := range patterns {
				matches := pattern.FindAllStringSubmatch(line, -1)
				for _, match := range matches {
					if len(match) > 1 {
						key := match[1]
						// Filter out common false positives
						if isValidTOMLKey(key) {
							keys[key] = append(keys[key], path)
						}
					}
				}
			}
		}

		return scanner.Err()
	})

	return keys, err
}

// isValidTOMLKey filters out false positives
func isValidTOMLKey(key string) bool {
	// Skip very short keys
	if len(key) < 2 {
		return false
	}

	// Skip common markdown/documentation keywords
	excludeList := []string{
		"md", "toml", "bash", "sh", "go", "json", "yaml",
		"true", "false", "yes", "no",
	}

	for _, exclude := range excludeList {
		if key == exclude {
			return false
		}
	}

	return true
}

// ComparisonReport contains the comparison results
type ComparisonReport struct {
	InCodeOnly   []*TOMLKey `json:"in_code_only"`
	InDocsOnly   []string   `json:"in_docs_only"`
	InBoth       []*TOMLKey `json:"in_both"`
	CodeKeyCount int        `json:"code_key_count"`
	DocKeyCount  int        `json:"doc_key_count"`
}

// compareKeys compares keys from code and documentation
func compareKeys(codeKeys map[string]*TOMLKey, docKeys map[string][]string) *ComparisonReport {
	report := &ComparisonReport{
		InCodeOnly:   []*TOMLKey{},
		InDocsOnly:   []string{},
		InBoth:       []*TOMLKey{},
		CodeKeyCount: len(codeKeys),
		DocKeyCount:  len(docKeys),
	}

	// Find keys in code
	for key, keyInfo := range codeKeys {
		if docFiles, exists := docKeys[key]; exists {
			keyInfo.DocFiles = docFiles
			report.InBoth = append(report.InBoth, keyInfo)
		} else {
			report.InCodeOnly = append(report.InCodeOnly, keyInfo)
		}
	}

	// Find keys only in docs
	for key := range docKeys {
		if _, exists := codeKeys[key]; !exists {
			report.InDocsOnly = append(report.InDocsOnly, key)
		}
	}

	// Sort for consistent output
	sort.Slice(report.InCodeOnly, func(i, j int) bool {
		return report.InCodeOnly[i].Key < report.InCodeOnly[j].Key
	})
	sort.Slice(report.InBoth, func(i, j int) bool {
		return report.InBoth[i].Key < report.InBoth[j].Key
	})
	sort.Strings(report.InDocsOnly)

	return report
}

// printReport prints the comparison report to stdout
func printReport(report *ComparisonReport, verbose bool) {
	fmt.Printf("=== TOML Configuration Key Verification Report ===\n\n")
	fmt.Printf("Total keys in code: %d\n", report.CodeKeyCount)
	fmt.Printf("Total keys in docs: %d\n", report.DocKeyCount)
	fmt.Printf("Keys in both: %d\n", len(report.InBoth))
	fmt.Printf("\n")

	if len(report.InCodeOnly) > 0 {
		fmt.Printf("⚠️  Keys in CODE but NOT in DOCS (%d):\n", len(report.InCodeOnly))
		for _, key := range report.InCodeOnly {
			fmt.Printf("  - %s (struct: %s, file: %s:%d)\n",
				key.Key, key.ParentStruct, filepath.Base(key.SourceFile), key.LineNumber)
			if verbose {
				fmt.Printf("    Type: %s\n", key.Type)
				fmt.Printf("    Tag: %s\n", key.StructTag)
			}
		}
		fmt.Printf("\n")
	}

	if len(report.InDocsOnly) > 0 {
		fmt.Printf("⚠️  Keys in DOCS but NOT in CODE (%d):\n", len(report.InDocsOnly))
		for _, key := range report.InDocsOnly {
			fmt.Printf("  - %s\n", key)
		}
		fmt.Printf("\n")
	}

	if verbose && len(report.InBoth) > 0 {
		fmt.Printf("✅ Keys documented correctly (%d):\n", len(report.InBoth))
		for _, key := range report.InBoth {
			fmt.Printf("  - %s\n", key.Key)
			for _, docFile := range key.DocFiles {
				fmt.Printf("    📄 %s\n", docFile)
			}
		}
		fmt.Printf("\n")
	}

	if len(report.InCodeOnly) == 0 && len(report.InDocsOnly) == 0 {
		fmt.Printf("✅ All keys are properly documented!\n")
	}
}

// writeJSONReport writes the report as JSON
func writeJSONReport(path string, report *ComparisonReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
