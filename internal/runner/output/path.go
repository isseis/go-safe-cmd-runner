package output

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// Path validation errors
var (
	ErrEmptyPath                 = errors.New("output path is empty")
	ErrWorkDirRequired           = errors.New("work directory is required for relative path")
	ErrPathTraversalAbsolute     = errors.New("path traversal detected in absolute path")
	ErrPathTraversalRelative     = errors.New("path traversal detected in relative path")
	ErrPathEscapesWorkDirectory  = errors.New("relative path escapes work directory")
	ErrDangerousCharactersInPath = errors.New("dangerous characters detected in path")
)

// DefaultPathValidator provides basic path validation without comprehensive security checks
// Security validation is handled separately by the SecurityValidator
type DefaultPathValidator struct{}

// NewDefaultPathValidator creates a new DefaultPathValidator
func NewDefaultPathValidator() *DefaultPathValidator {
	return &DefaultPathValidator{}
}

// ValidateAndResolvePath validates and resolves an output path
// This performs basic path validation and path traversal prevention
// Additional security checks (symlink detection, etc.) are performed by SecurityValidator
func (v *DefaultPathValidator) ValidateAndResolvePath(outputPath, workDir string) (string, error) {
	// Trim whitespace and check for empty path
	if outputPath == "" {
		return "", ErrEmptyPath
	}

	if filepath.IsAbs(outputPath) {
		return v.validateAbsolutePath(outputPath)
	}

	return v.validateRelativePath(outputPath, workDir)
}

// validateAbsolutePath validates and cleans an absolute path
func (v *DefaultPathValidator) validateAbsolutePath(path string) (string, error) {
	// Check for path traversal patterns by examining path segments
	if containsPathTraversalSegment(path) {
		return "", fmt.Errorf("%w: %s", ErrPathTraversalAbsolute, path)
	}

	// Check for dangerous characters in the path
	if chars := containsDangerousCharacters(path); len(chars) > 0 {
		return "", fmt.Errorf("%w: %s (found: %v)", ErrDangerousCharactersInPath, path, chars)
	}

	// Clean the path to remove redundant separators and resolve . components
	cleanPath := filepath.Clean(path)
	return cleanPath, nil
}

// validateRelativePath validates and resolves a relative path within workDir
func (v *DefaultPathValidator) validateRelativePath(path, workDir string) (string, error) {
	if workDir == "" {
		return "", ErrWorkDirRequired
	}

	// Check for path traversal patterns by examining path segments
	if containsPathTraversalSegment(path) {
		return "", fmt.Errorf("%w: %s", ErrPathTraversalRelative, path)
	}

	// Check for dangerous characters in the path
	if chars := containsDangerousCharacters(path); len(chars) > 0 {
		return "", fmt.Errorf("%w: %s (found: %v)", ErrDangerousCharactersInPath, path, chars)
	}

	// Join with work directory and clean
	fullPath := filepath.Join(workDir, path)
	cleanPath := filepath.Clean(fullPath)

	// Ensure the result is still within the work directory
	cleanWorkDir := filepath.Clean(workDir)

	// Use filepath.Rel to check if the path escapes the work directory
	relPath, err := filepath.Rel(cleanWorkDir, cleanPath)
	if err != nil || escapesWorkDirectory(relPath) {
		return "", fmt.Errorf("%w: %s", ErrPathEscapesWorkDirectory, path)
	}

	return cleanPath, nil
}

// containsPathTraversalSegment checks if a path contains ".." as a distinct path segment
// This avoids false positives for legitimate filenames that contain ".." (e.g., "archive..zip")
func containsPathTraversalSegment(path string) bool {
	// Split the path into segments and check each one
	for _, segment := range strings.Split(path, string(filepath.Separator)) {
		if segment == ".." {
			return true
		}
	}
	return false
}

// escapesWorkDirectory checks if a relative path escapes the work directory
// This checks for path segments that would escape, avoiding false positives
// for filenames that start with ".." (e.g., "..hidden-file")
func escapesWorkDirectory(relPath string) bool {
	if relPath == ".." {
		return true
	}

	// Check if the path starts with a ".." segment followed by separator
	// This correctly identifies "../file" but not "..hidden-file"
	return strings.HasPrefix(relPath, ".."+string(filepath.Separator))
}

// containsDangerousCharacters checks if a path contains characters that could be
// problematic when processing the path in shell scripts or command-line tools
// Returns a slice of the dangerous characters found (empty if none)
func containsDangerousCharacters(path string) []string {
	var found []string
	foundMap := make(map[string]bool) // To avoid duplicates

	// Define dangerous character patterns
	// High-risk: Shell metacharacters that can cause command injection
	highRiskChars := []string{
		";", "&", "|", "$", "`", ">", "<", "&&", "||", "$(", "${", ">>", "<<",
	}

	// Medium-risk: Characters that can cause unintended shell expansion
	// Note: Space and tab are handled separately with unicode.IsSpace
	mediumRiskChars := []string{
		"*", "?", "[", "]", "~", "!",
	}

	// Low-risk: Control characters and other potentially problematic characters
	lowRiskChars := []string{
		"\n", "\r", "\f", "\v", "\b", "\a", "\\",
	}

	// High-risk currency symbols that might be confused with shell variables
	highRiskSymbols := []rune{'€', '¥', '£', '¢', '₹', '₽', '₩'}

	// Helper function to add unique dangerous characters
	addDangerous := func(char string) {
		if !foundMap[char] {
			found = append(found, char)
			foundMap[char] = true
		}
	}

	// Check for multi-character patterns first
	for _, char := range highRiskChars {
		if strings.Contains(path, char) {
			addDangerous(char)
		}
	}

	// Check for single-character patterns
	for _, char := range mediumRiskChars {
		if strings.Contains(path, char) {
			addDangerous(char)
		}
	}

	// Check for control characters and other low-risk patterns
	for _, char := range lowRiskChars {
		if strings.Contains(path, char) {
			addDangerous(char)
		}
	}

	// Check each rune for various dangerous categories
	for _, r := range path {
		runeStr := string(r)

		// Skip if already found
		if foundMap[runeStr] {
			continue
		}

		// Check for space characters using unicode.IsSpace (comprehensive)
		if unicode.IsSpace(r) {
			addDangerous(runeStr)
			continue
		}

		// Check for high-risk symbol characters
		if unicode.IsSymbol(r) && slices.Contains(highRiskSymbols, r) {
			addDangerous(runeStr)
			continue
		}

		// Check for other control characters not in lowRiskChars
		if unicode.IsControl(r) && !slices.Contains(lowRiskChars, runeStr) {
			addDangerous(runeStr)
		}
	}

	return found
}
