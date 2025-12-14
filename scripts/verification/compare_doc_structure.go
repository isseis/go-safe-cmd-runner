//go:build ignore

// compare_doc_structure.go - Compare structure between Japanese and English docs
//
// This script compares the structure (headings, sections) between
// Japanese (.ja.md) and English (.md) documentation files.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Heading represents a markdown heading
type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Line  int    `json:"line"`
}

// DocStructure represents the structure of a documentation file
type DocStructure struct {
	FilePath   string    `json:"file_path"`
	Language   string    `json:"language"`
	Headings   []Heading `json:"headings"`
	CodeBlocks int       `json:"code_blocks"`
	Lists      int       `json:"lists"`
	Tables     int       `json:"tables"`
	LineCount  int       `json:"line_count"`
}

// StructureComparison represents a comparison between two documents
type StructureComparison struct {
	JaFile         string         `json:"ja_file"`
	EnFile         string         `json:"en_file"`
	JaStructure    *DocStructure  `json:"ja_structure"`
	EnStructure    *DocStructure  `json:"en_structure"`
	HeadingMatches []HeadingMatch `json:"heading_matches"`
	Mismatches     []string       `json:"mismatches"`
	MissingInJa    []string       `json:"missing_in_ja"`
	MissingInEn    []string       `json:"missing_in_en"`
}

// HeadingMatch represents a matched heading pair
type HeadingMatch struct {
	Level   int    `json:"level"`
	JaText  string `json:"ja_text"`
	EnText  string `json:"en_text"`
	JaLine  int    `json:"ja_line"`
	EnLine  int    `json:"en_line"`
	IsMatch bool   `json:"is_match"`
}

// Config holds the script configuration
type Config struct {
	DocsRoot   string
	OutputJSON string
	Verbose    bool
}

func main() {
	config := parseFlags()

	// Find all Japanese documentation files
	jaFiles, err := findJapaneseFiles(config.DocsRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding files: %v\n", err)
		os.Exit(1)
	}

	// Compare each Japanese file with its English counterpart
	comparisons := []StructureComparison{}
	for _, jaFile := range jaFiles {
		enFile := getEnglishCounterpart(jaFile)

		comparison, err := compareFiles(jaFile, enFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error comparing %s: %v\n", jaFile, err)
			continue
		}

		comparisons = append(comparisons, *comparison)
	}

	// Print report
	printReport(comparisons, config.Verbose)

	// Write JSON if requested
	if config.OutputJSON != "" {
		if err := writeJSONReport(config.OutputJSON, comparisons); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON report: %v\n", err)
			os.Exit(1)
		}
	}
}

func parseFlags() *Config {
	config := &Config{}
	flag.StringVar(&config.DocsRoot, "docs", "docs/user", "Root directory of documentation")
	flag.StringVar(&config.OutputJSON, "output", "", "Output JSON file path (optional)")
	flag.BoolVar(&config.Verbose, "verbose", false, "Verbose output")
	flag.Parse()
	return config
}

// findJapaneseFiles finds all Japanese documentation files
func findJapaneseFiles(rootDir string) ([]string, error) {
	var files []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".ja.md") {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// getEnglishCounterpart returns the English version filename
func getEnglishCounterpart(jaFile string) string {
	return strings.Replace(jaFile, ".ja.md", ".md", 1)
}

// compareFiles compares structure between Japanese and English files
func compareFiles(jaFile, enFile string) (*StructureComparison, error) {
	// Parse both files
	jaStructure, err := parseDocStructure(jaFile, "ja")
	if err != nil {
		return nil, fmt.Errorf("parsing Japanese file: %w", err)
	}

	var enStructure *DocStructure
	enExists := true
	enStructure, err = parseDocStructure(enFile, "en")
	if err != nil {
		if os.IsNotExist(err) {
			enExists = false
			enStructure = &DocStructure{FilePath: enFile, Language: "en"}
		} else {
			return nil, fmt.Errorf("parsing English file: %w", err)
		}
	}

	comparison := &StructureComparison{
		JaFile:      jaFile,
		EnFile:      enFile,
		JaStructure: jaStructure,
		EnStructure: enStructure,
		Mismatches:  []string{},
		MissingInJa: []string{},
		MissingInEn: []string{},
	}

	if !enExists {
		comparison.Mismatches = append(comparison.Mismatches, "English file does not exist")
		return comparison, nil
	}

	// Compare headings
	comparison.HeadingMatches = compareHeadings(jaStructure.Headings, enStructure.Headings)

	// Check for structural differences
	if len(jaStructure.Headings) != len(enStructure.Headings) {
		comparison.Mismatches = append(comparison.Mismatches,
			fmt.Sprintf("Different number of headings: JA=%d, EN=%d",
				len(jaStructure.Headings), len(enStructure.Headings)))
	}

	if jaStructure.CodeBlocks != enStructure.CodeBlocks {
		comparison.Mismatches = append(comparison.Mismatches,
			fmt.Sprintf("Different number of code blocks: JA=%d, EN=%d",
				jaStructure.CodeBlocks, enStructure.CodeBlocks))
	}

	if jaStructure.Tables != enStructure.Tables {
		comparison.Mismatches = append(comparison.Mismatches,
			fmt.Sprintf("Different number of tables: JA=%d, EN=%d",
				jaStructure.Tables, enStructure.Tables))
	}

	// Find headings that appear in one file but not the other
	jaHeadingMap := make(map[int]map[string]bool)
	enHeadingMap := make(map[int]map[string]bool)

	for _, h := range jaStructure.Headings {
		if jaHeadingMap[h.Level] == nil {
			jaHeadingMap[h.Level] = make(map[string]bool)
		}
		jaHeadingMap[h.Level][normalizeHeading(h.Text)] = true
	}

	for _, h := range enStructure.Headings {
		if enHeadingMap[h.Level] == nil {
			enHeadingMap[h.Level] = make(map[string]bool)
		}
		normalized := normalizeHeading(h.Text)
		enHeadingMap[h.Level][normalized] = true

		// Check if this heading exists in Japanese
		if !jaHeadingMap[h.Level][normalized] {
			comparison.MissingInJa = append(comparison.MissingInJa,
				fmt.Sprintf("Level %d: %s", h.Level, h.Text))
		}
	}

	for _, h := range jaStructure.Headings {
		normalized := normalizeHeading(h.Text)
		if !enHeadingMap[h.Level][normalized] {
			comparison.MissingInEn = append(comparison.MissingInEn,
				fmt.Sprintf("Level %d: %s", h.Level, h.Text))
		}
	}

	return comparison, nil
}

// parseDocStructure parses the structure of a documentation file
func parseDocStructure(filePath, lang string) (*DocStructure, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	structure := &DocStructure{
		FilePath: filePath,
		Language: lang,
		Headings: []Heading{},
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0
	inCodeBlock := false

	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	tableRegex := regexp.MustCompile(`^\|.*\|$`)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		structure.LineCount = lineNum

		// Track code blocks
		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				structure.CodeBlocks++
				inCodeBlock = true
			} else {
				inCodeBlock = false
			}
			continue
		}

		if inCodeBlock {
			continue
		}

		// Parse headings
		if matches := headingRegex.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			text := strings.TrimSpace(matches[2])
			structure.Headings = append(structure.Headings, Heading{
				Level: level,
				Text:  text,
				Line:  lineNum,
			})
			continue
		}

		// Count tables
		if tableRegex.MatchString(line) {
			// Simple heuristic: if we see a table row, count it as one table
			// (not perfect but good enough for structure comparison)
			if lineNum > 1 {
				prevWasTable := false
				// This is a simplified check; a real implementation would track table state
				if !prevWasTable {
					structure.Tables++
				}
			}
		}

		// Count lists (simple heuristic)
		if strings.HasPrefix(strings.TrimSpace(line), "- ") ||
			strings.HasPrefix(strings.TrimSpace(line), "* ") ||
			regexp.MustCompile(`^\s*\d+\.\s`).MatchString(line) {
			structure.Lists++
		}
	}

	return structure, scanner.Err()
}

// compareHeadings compares headings between two documents
func compareHeadings(jaHeadings, enHeadings []Heading) []HeadingMatch {
	matches := []HeadingMatch{}

	maxLen := len(jaHeadings)
	if len(enHeadings) > maxLen {
		maxLen = len(enHeadings)
	}

	for i := 0; i < maxLen; i++ {
		match := HeadingMatch{}

		if i < len(jaHeadings) {
			match.Level = jaHeadings[i].Level
			match.JaText = jaHeadings[i].Text
			match.JaLine = jaHeadings[i].Line
		}

		if i < len(enHeadings) {
			if match.Level == 0 {
				match.Level = enHeadings[i].Level
			}
			match.EnText = enHeadings[i].Text
			match.EnLine = enHeadings[i].Line
		}

		// Check if levels match
		if i < len(jaHeadings) && i < len(enHeadings) {
			match.IsMatch = jaHeadings[i].Level == enHeadings[i].Level
		}

		matches = append(matches, match)
	}

	return matches
}

// normalizeHeading normalizes heading text for comparison
func normalizeHeading(text string) string {
	// Remove common punctuation and normalize whitespace
	text = strings.ToLower(text)
	text = strings.TrimSpace(text)
	// Remove trailing periods, colons, etc.
	text = strings.TrimRight(text, ".:")
	return text
}

// printReport prints the comparison report
func printReport(comparisons []StructureComparison, verbose bool) {
	fmt.Printf("=== Document Structure Comparison Report ===\n\n")
	fmt.Printf("Total files compared: %d\n\n", len(comparisons))

	filesWithIssues := 0
	for _, comp := range comparisons {
		hasIssues := len(comp.Mismatches) > 0 || len(comp.MissingInJa) > 0 || len(comp.MissingInEn) > 0

		if !verbose && !hasIssues {
			continue
		}

		if hasIssues {
			filesWithIssues++
		}

		fmt.Printf("📄 %s\n", filepath.Base(comp.JaFile))

		if len(comp.Mismatches) > 0 {
			fmt.Printf("  ⚠️  Structure Mismatches:\n")
			for _, mismatch := range comp.Mismatches {
				fmt.Printf("    - %s\n", mismatch)
			}
		}

		if len(comp.MissingInJa) > 0 {
			fmt.Printf("  ⚠️  Missing in Japanese (but in English):\n")
			for _, missing := range comp.MissingInJa {
				fmt.Printf("    - %s\n", missing)
			}
		}

		if len(comp.MissingInEn) > 0 {
			fmt.Printf("  ⚠️  Missing in English (but in Japanese):\n")
			for _, missing := range comp.MissingInEn {
				fmt.Printf("    - %s\n", missing)
			}
		}

		if verbose {
			fmt.Printf("  ℹ️  Statistics:\n")
			fmt.Printf("    JA: %d headings, %d code blocks, %d tables, %d lines\n",
				len(comp.JaStructure.Headings), comp.JaStructure.CodeBlocks,
				comp.JaStructure.Tables, comp.JaStructure.LineCount)
			if comp.EnStructure.LineCount > 0 {
				fmt.Printf("    EN: %d headings, %d code blocks, %d tables, %d lines\n",
					len(comp.EnStructure.Headings), comp.EnStructure.CodeBlocks,
					comp.EnStructure.Tables, comp.EnStructure.LineCount)
			}
		}

		if !hasIssues {
			fmt.Printf("  ✅ Structure matches\n")
		}

		fmt.Printf("\n")
	}

	if filesWithIssues == 0 {
		fmt.Printf("✅ All documents have matching structure!\n")
	} else {
		fmt.Printf("⚠️  Found issues in %d/%d files\n", filesWithIssues, len(comparisons))
	}
}

// writeJSONReport writes the report as JSON
func writeJSONReport(path string, comparisons []StructureComparison) error {
	data, err := json.MarshalIndent(comparisons, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
