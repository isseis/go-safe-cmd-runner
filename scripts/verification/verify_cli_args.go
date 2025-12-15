//go:build ignore

// verify_cli_args.go - Extract and verify command-line arguments
//
// This script extracts command-line arguments from Go source code
// (using flag package) and compares them with arguments documented
// in Japanese documentation files.

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

// CLIArg represents a command-line argument
type CLIArg struct {
	Name         string   `json:"name"`
	ShortForm    string   `json:"short_form,omitempty"`
	Type         string   `json:"type"`
	DefaultValue string   `json:"default_value,omitempty"`
	Description  string   `json:"description,omitempty"`
	SourceFile   string   `json:"source_file"`
	LineNumber   int      `json:"line_number"`
	Command      string   `json:"command"`
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

	// Extract CLI args from Go source code
	codeArgs, err := extractArgsFromCode(config.SourceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting args from code: %v\n", err)
		os.Exit(1)
	}

	// Extract CLI args from documentation
	docArgs, err := extractArgsFromDocs(config.DocsRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting args from docs: %v\n", err)
		os.Exit(1)
	}

	// Compare and report
	report := compareArgs(codeArgs, docArgs)

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
	flag.StringVar(&config.SourceRoot, "source", "cmd", "Root directory of command source code")
	flag.StringVar(&config.DocsRoot, "docs", "docs/user", "Root directory of documentation")
	flag.StringVar(&config.OutputJSON, "output", "", "Output JSON file path (optional)")
	flag.BoolVar(&config.Verbose, "verbose", false, "Verbose output")
	flag.Parse()
	return config
}

// extractArgsFromCode extracts CLI arguments from flag package calls
func extractArgsFromCode(rootDir string) (map[string]*CLIArg, error) {
	args := make(map[string]*CLIArg)
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

		// Determine command name from path
		cmdName := extractCommandName(path)

		ast.Inspect(file, func(n ast.Node) bool {
			callExpr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Look for flag.String, flag.Bool, flag.Int, etc.
			selector, ok := callExpr.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := selector.X.(*ast.Ident)
			if !ok || ident.Name != "flag" {
				return true
			}

			funcName := selector.Sel.Name
			flagType := extractFlagType(funcName)
			if flagType == "" {
				return true
			}

			// Extract arguments: name, defaultValue, description
			if len(callExpr.Args) < 3 {
				return true
			}

			flagName := extractStringLiteral(callExpr.Args[0])
			defaultValue := extractLiteralValue(callExpr.Args[1])
			description := extractStringLiteral(callExpr.Args[2])

			if flagName == "" {
				return true
			}

			position := fset.Position(callExpr.Pos())
			arg := &CLIArg{
				Name:         flagName,
				Type:         flagType,
				DefaultValue: defaultValue,
				Description:  description,
				SourceFile:   path,
				LineNumber:   position.Line,
				Command:      cmdName,
			}

			key := cmdName + ":" + flagName
			args[key] = arg

			return true
		})

		return nil
	})

	return args, err
}

// extractCommandName extracts command name from file path
func extractCommandName(path string) string {
	// cmd/runner/main.go -> runner
	// cmd/record/main.go -> record
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range parts {
		if part == "cmd" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "unknown"
}

// extractFlagType extracts flag type from function name
func extractFlagType(funcName string) string {
	typeMap := map[string]string{
		"String":   "string",
		"Bool":     "bool",
		"Int":      "int",
		"Int64":    "int64",
		"Uint":     "uint",
		"Uint64":   "uint64",
		"Float64":  "float64",
		"Duration": "duration",
	}
	return typeMap[funcName]
}

// extractStringLiteral extracts string value from ast.Expr
func extractStringLiteral(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	// Remove quotes
	value := lit.Value
	if len(value) >= 2 {
		return value[1 : len(value)-1]
	}
	return ""
}

// extractLiteralValue extracts any literal value as string
func extractLiteralValue(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		return v.Value
	case *ast.Ident:
		return v.Name
	default:
		return fmt.Sprintf("%v", expr)
	}
}

// extractArgsFromDocs extracts CLI arguments from documentation
func extractArgsFromDocs(docsRoot string) (map[string][]string, error) {
	args := make(map[string][]string)

	// Patterns to match CLI arguments in documentation
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`-([a-z])\s+`),   // -c, -n (short form)
		regexp.MustCompile(`--([a-z-]+)`),   // --config, --dry-run
		regexp.MustCompile("`-([a-z])`"),    // `-c` in text
		regexp.MustCompile("`--([a-z-]+)`"), // `--config` in text
	}

	err := filepath.Walk(docsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".ja.md") {
			return nil
		}

		// Determine command from filename
		cmdName := extractCommandFromFilename(path)

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()

			for _, pattern := range patterns {
				matches := pattern.FindAllStringSubmatch(line, -1)
				for _, match := range matches {
					if len(match) > 1 {
						argName := match[1]
						if isValidArgName(argName) {
							key := cmdName + ":" + argName
							args[key] = append(args[key], path)
						}
					}
				}
			}
		}

		return scanner.Err()
	})

	return args, err
}

// extractCommandFromFilename extracts command name from documentation filename
func extractCommandFromFilename(path string) string {
	// docs/user/runner_command.ja.md -> runner
	// docs/user/record_command.ja.md -> record
	filename := filepath.Base(path)
	if strings.HasSuffix(filename, "_command.ja.md") {
		return strings.TrimSuffix(filename, "_command.ja.md")
	}
	return "unknown"
}

// isValidArgName filters out false positives
func isValidArgName(name string) bool {
	// Skip very short names (except valid single-letter flags)
	if len(name) == 0 {
		return false
	}

	// Common false positives to exclude from tracking
	excludeList := []string{
		"h", "help", "version", "v",
	}

	for _, exclude := range excludeList {
		if name == exclude {
			return false // Filter out these common arguments
		}
	}

	return true
}

// ComparisonReport contains the comparison results
type ComparisonReport struct {
	InCodeOnly   []*CLIArg `json:"in_code_only"`
	InDocsOnly   []string  `json:"in_docs_only"`
	InBoth       []*CLIArg `json:"in_both"`
	CodeArgCount int       `json:"code_arg_count"`
	DocArgCount  int       `json:"doc_arg_count"`
}

// compareArgs compares arguments from code and documentation
func compareArgs(codeArgs map[string]*CLIArg, docArgs map[string][]string) *ComparisonReport {
	report := &ComparisonReport{
		InCodeOnly:   []*CLIArg{},
		InDocsOnly:   []string{},
		InBoth:       []*CLIArg{},
		CodeArgCount: len(codeArgs),
		DocArgCount:  len(docArgs),
	}

	// Find args in code
	for key, argInfo := range codeArgs {
		if docFiles, exists := docArgs[key]; exists {
			argInfo.DocFiles = docFiles
			report.InBoth = append(report.InBoth, argInfo)
		} else {
			report.InCodeOnly = append(report.InCodeOnly, argInfo)
		}
	}

	// Find args only in docs
	for key := range docArgs {
		if _, exists := codeArgs[key]; !exists {
			report.InDocsOnly = append(report.InDocsOnly, key)
		}
	}

	// Sort for consistent output
	sort.Slice(report.InCodeOnly, func(i, j int) bool {
		return report.InCodeOnly[i].Command+":"+report.InCodeOnly[i].Name <
			report.InCodeOnly[j].Command+":"+report.InCodeOnly[j].Name
	})
	sort.Slice(report.InBoth, func(i, j int) bool {
		return report.InBoth[i].Command+":"+report.InBoth[i].Name <
			report.InBoth[j].Command+":"+report.InBoth[j].Name
	})
	sort.Strings(report.InDocsOnly)

	return report
}

// printReport prints the comparison report to stdout
func printReport(report *ComparisonReport, verbose bool) {
	fmt.Printf("=== Command-Line Argument Verification Report ===\n\n")
	fmt.Printf("Total arguments in code: %d\n", report.CodeArgCount)
	fmt.Printf("Total arguments in docs: %d\n", report.DocArgCount)
	fmt.Printf("Arguments in both: %d\n", len(report.InBoth))
	fmt.Printf("\n")

	if len(report.InCodeOnly) > 0 {
		fmt.Printf("⚠️  Arguments in CODE but NOT in DOCS (%d):\n", len(report.InCodeOnly))
		for _, arg := range report.InCodeOnly {
			fmt.Printf("  - %s: -%s (%s)\n", arg.Command, arg.Name, arg.Type)
			if verbose {
				fmt.Printf("    Default: %s\n", arg.DefaultValue)
				fmt.Printf("    Description: %s\n", arg.Description)
				fmt.Printf("    Source: %s:%d\n", filepath.Base(arg.SourceFile), arg.LineNumber)
			}
		}
		fmt.Printf("\n")
	}

	if len(report.InDocsOnly) > 0 {
		fmt.Printf("⚠️  Arguments in DOCS but NOT in CODE (%d):\n", len(report.InDocsOnly))
		for _, key := range report.InDocsOnly {
			fmt.Printf("  - %s\n", key)
		}
		fmt.Printf("\n")
	}

	if verbose && len(report.InBoth) > 0 {
		fmt.Printf("✅ Arguments documented correctly (%d):\n", len(report.InBoth))
		for _, arg := range report.InBoth {
			fmt.Printf("  - %s: -%s (%s)\n", arg.Command, arg.Name, arg.Type)
			for _, docFile := range arg.DocFiles {
				fmt.Printf("    📄 %s\n", filepath.Base(docFile))
			}
		}
		fmt.Printf("\n")
	}

	if len(report.InCodeOnly) == 0 && len(report.InDocsOnly) == 0 {
		fmt.Printf("✅ All arguments are properly documented!\n")
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
