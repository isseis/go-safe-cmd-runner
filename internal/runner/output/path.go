package output

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Path validation errors
var (
	ErrEmptyPath                 = errors.New("output path is empty")
	ErrWorkDirRequired           = errors.New("work directory is required for relative path")
	ErrPathTraversal             = errors.New("path traversal detected")
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
	// Check for empty path
	if outputPath == "" {
		return "", ErrEmptyPath
	}

	// Perform common security validation
	if err := validatePathSecurity(outputPath); err != nil {
		return "", err
	}

	if filepath.IsAbs(outputPath) {
		return v.validateAbsolutePath(outputPath)
	}

	return v.validateRelativePath(outputPath, workDir)
}

// validatePathSecurity performs common security validation for both absolute and relative paths
func validatePathSecurity(path string) error {
	// Check for path traversal patterns by examining path segments
	if ContainsPathTraversalSegment(path) {
		return fmt.Errorf("%w: %s", ErrPathTraversal, path)
	}

	// Check for dangerous characters in the path
	if chars := containsDangerousCharacters(path); len(chars) > 0 {
		return fmt.Errorf("%w: %s (found: %v)", ErrDangerousCharactersInPath, path, chars)
	}

	return nil
}

// validateAbsolutePath validates and cleans an absolute path
func (v *DefaultPathValidator) validateAbsolutePath(path string) (string, error) {
	// Clean the path to remove redundant separators and resolve . components
	cleanPath := filepath.Clean(path)
	return cleanPath, nil
}

// validateRelativePath validates and resolves a relative path within workDir
func (v *DefaultPathValidator) validateRelativePath(path, workDir string) (string, error) {
	if workDir == "" {
		return "", ErrWorkDirRequired
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

// ContainsPathTraversalSegment checks if a path contains ".." as a distinct path segment
// This avoids false positives for legitimate filenames that contain ".." (e.g., "archive..zip")
func ContainsPathTraversalSegment(path string) bool {
	// Split the path into segments and check if any segment is ".."
	segments := strings.Split(path, string(filepath.Separator))
	return slices.Contains(segments, "..")
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

// dangerousCharLookup contains all single dangerous characters for fast lookup
var dangerousCharLookup = map[rune]bool{
	// Shell metacharacters
	';': true, '&': true, '|': true, '$': true, '`': true,
	'>': true, '<': true,
	// Glob/expansion characters
	'*': true, '?': true, '[': true, ']': true, '~': true, '!': true,
	// Control characters
	'\n': true, '\r': true, '\f': true, '\v': true, '\b': true, '\a': true, '\\': true,
}

// dangerousSymbolLookup contains high-risk currency symbols
var dangerousSymbolLookup = map[rune]bool{
	'€': true, '¥': true, '£': true, '¢': true, '₹': true, '₽': true, '₩': true,
}

// multiCharPatternRegex is a pre-compiled regex for detecting multi-character dangerous patterns
var multiCharPatternRegex = regexp.MustCompile(`&&|\|\||\$\(|\$\{|>>|<<`)

// containsDangerousCharacters checks if a path contains characters that could be
// problematic when processing the path in shell scripts or command-line tools
// Returns a slice of the dangerous characters found (empty if none)
// Optimized version that scans the path only once with efficient lookups
func containsDangerousCharacters(path string) []string {
	var found []string
	foundMap := make(map[string]bool) // To avoid duplicates

	// Helper function to add unique dangerous characters
	addDangerous := func(char string) {
		if !foundMap[char] {
			found = append(found, char)
			foundMap[char] = true
		}
	}

	// Check for multi-character patterns using pre-compiled regex
	matches := multiCharPatternRegex.FindAllString(path, -1)
	for _, match := range matches {
		addDangerous(match)
	}

	// Single-pass scan through all runes for single-character patterns
	for _, r := range path {
		runeStr := string(r)

		// Skip if already found
		if foundMap[runeStr] {
			continue
		}

		// Check single-character lookup table first (most common case)
		if dangerousCharLookup[r] {
			addDangerous(runeStr)
			continue
		}

		// Check for space characters using unicode.IsSpace (comprehensive)
		if unicode.IsSpace(r) {
			addDangerous(runeStr)
			continue
		}

		// Check for high-risk symbol characters
		if unicode.IsSymbol(r) && dangerousSymbolLookup[r] {
			addDangerous(runeStr)
			continue
		}

		// Check for other control characters not in lookup table
		if unicode.IsControl(r) {
			addDangerous(runeStr)
		}
	}

	return found
}
