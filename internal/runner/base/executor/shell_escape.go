package executor

import (
	"strings"
)

// ShellEscape escapes a string for safe use in shell commands
// Returns a properly quoted string that can be copy-pasted into a shell
func ShellEscape(s string) string {
	// If string is empty, return empty quotes
	if s == "" {
		return "''"
	}

	// Check if string needs quoting
	// Safe characters: alphanumeric, -, _, ., /, :, @
	needsQuoting := false
	for _, r := range s {
		if !isSafeChar(r) {
			needsQuoting = true
			break
		}
	}

	// If no special characters, return as-is
	if !needsQuoting {
		return s
	}

	// Use single quotes and escape any single quotes in the string
	// Replace ' with '\'' (end quote, escaped quote, start quote)
	escaped := strings.ReplaceAll(s, "'", "'\\''")
	return "'" + escaped + "'"
}

// isSafeChar checks if a character is safe to use in shell without quoting
func isSafeChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '.' || r == '/' || r == ':' || r == '@'
}

// FormatCommandForLog formats a command with arguments for logging
// Returns a string that can be copy-pasted into a shell
func FormatCommandForLog(path string, args []string) string {
	parts := make([]string, 1+len(args))
	parts[0] = ShellEscape(path)
	for i, arg := range args {
		parts[i+1] = ShellEscape(arg)
	}
	return strings.Join(parts, " ")
}
