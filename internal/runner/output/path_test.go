//go:build test

package output

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPathValidator_ValidateAndResolvePath(t *testing.T) {
	tests := []struct {
		name        string
		outputPath  string
		workDir     string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid_absolute_path",
			outputPath: "/tmp/output.txt",
			workDir:    "/home/user",
			wantErr:    false,
		},
		{
			name:       "valid_relative_path",
			outputPath: "output/result.txt",
			workDir:    "/home/user/project",
			wantErr:    false,
		},
		{
			name:        "path_traversal_absolute",
			outputPath:  "/tmp/../etc/passwd",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:        "path_traversal_relative",
			outputPath:  "../../../etc/passwd",
			workDir:     "/home/user/project",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:        "double_dot_in_middle_absolute",
			outputPath:  "/home/../etc/passwd",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:        "double_dot_in_middle_relative",
			outputPath:  "safe/../../../unsafe",
			workDir:     "/home/user/project",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:        "empty_path",
			outputPath:  "",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "output path is empty",
		},
		{
			name:        "dangerous_chars_semicolon",
			outputPath:  "/tmp/file;rm.txt",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "dangerous characters detected",
		},
		{
			name:        "dangerous_chars_pipe",
			outputPath:  "/tmp/file|evil.txt",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "dangerous characters detected",
		},
		{
			name:        "dangerous_chars_space",
			outputPath:  "/tmp/my file.txt",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "dangerous characters detected",
		},
		{
			name:        "dangerous_chars_wildcard",
			outputPath:  "/tmp/file*.txt",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "dangerous characters detected",
		},
		{
			name:        "dangerous_chars_dollar",
			outputPath:  "/tmp/file$var.txt",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "dangerous characters detected",
		},
		{
			name:        "dangerous_chars_backtick",
			outputPath:  "/tmp/file`cmd`.txt",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "dangerous characters detected",
		},
		{
			name:        "dangerous_chars_newline",
			outputPath:  "/tmp/file\ntest.txt",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "dangerous characters detected",
		},
		{
			name:        "dangerous_chars_unicode_space",
			outputPath:  "/tmp/file\u00a0test.txt", // Non-breaking space
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "dangerous characters detected",
		},
		{
			name:        "dangerous_chars_currency_symbol",
			outputPath:  "/tmp/file€test.txt",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "dangerous characters detected",
		},
		{
			name:       "safe_symbols_copyright",
			outputPath: "/tmp/file©test.txt", // Copyright symbol should be safe
			workDir:    "/home/user",
			wantErr:    false,
		},
		{
			name:        "relative_path_without_workdir",
			outputPath:  "output.txt",
			workDir:     "",
			wantErr:     true,
			errContains: "work directory is required",
		},
		{
			name:        "relative_path_escapes_workdir",
			outputPath:  "../../../../etc/passwd",
			workDir:     "/home/user/project",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:       "valid_relative_path_with_subdirs",
			outputPath: "logs/output/result.txt",
			workDir:    "/home/user/project",
			wantErr:    false,
		},
		{
			name:       "valid_absolute_path_with_subdirs",
			outputPath: "/tmp/safe/output/result.txt",
			workDir:    "/home/user",
			wantErr:    false,
		},
	}

	validator := NewDefaultPathValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.ValidateAndResolvePath(tt.outputPath, tt.workDir)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				assert.True(t, filepath.IsAbs(result))

				// Verify the path is clean (no redundant separators, etc.)
				assert.Equal(t, filepath.Clean(result), result)

				// For relative paths, verify they are within workDir
				if !filepath.IsAbs(tt.outputPath) {
					cleanWorkDir := filepath.Clean(tt.workDir)
					assert.True(t, strings.HasPrefix(result, cleanWorkDir))
				}
			}
		})
	}
}

func TestDefaultPathValidator_validateAbsolutePath(t *testing.T) {
	validator := NewDefaultPathValidator()

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid_absolute_path",
			path:    "/tmp/output.txt",
			wantErr: false,
		},
		{
			name:    "path_with_single_dot",
			path:    "/tmp/./output.txt",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.validateAbsolutePath(tt.path)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				assert.Equal(t, filepath.Clean(tt.path), result)
			}
		})
	}
}

func TestDefaultPathValidator_validateRelativePath(t *testing.T) {
	validator := NewDefaultPathValidator()

	tests := []struct {
		name        string
		path        string
		workDir     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid_relative_path",
			path:    "output.txt",
			workDir: "/home/user",
			wantErr: false,
		},
		{
			name:    "valid_relative_path_with_subdirs",
			path:    "logs/output.txt",
			workDir: "/home/user/project",
			wantErr: false,
		},
		{
			name:        "empty_workdir",
			path:        "output.txt",
			workDir:     "",
			wantErr:     true,
			errContains: "work directory is required",
		},
		{
			name:    "relative_path_with_current_dir",
			path:    "./output.txt",
			workDir: "/home/user",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.validateRelativePath(tt.path, tt.workDir)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				assert.True(t, filepath.IsAbs(result))

				// Verify the path is within workDir
				cleanWorkDir := filepath.Clean(tt.workDir)
				assert.True(t, strings.HasPrefix(result, cleanWorkDir))
			}
		})
	}
}

func TestDefaultPathValidator_EdgeCases(t *testing.T) {
	validator := NewDefaultPathValidator()

	t.Run("workdir_with_trailing_slash", func(t *testing.T) {
		result, err := validator.ValidateAndResolvePath("output.txt", "/home/user/")
		require.NoError(t, err)
		assert.Equal(t, "/home/user/output.txt", result)
	})

	t.Run("path_with_multiple_slashes", func(t *testing.T) {
		result, err := validator.ValidateAndResolvePath("//tmp///output.txt", "/home/user")
		require.NoError(t, err)
		assert.Equal(t, "/tmp/output.txt", result)
	})

	t.Run("relative_path_with_multiple_slashes", func(t *testing.T) {
		result, err := validator.ValidateAndResolvePath("logs//output.txt", "/home/user")
		require.NoError(t, err)
		assert.Equal(t, "/home/user/logs/output.txt", result)
	})
}

func TestContainsDangerousCharacters(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string // Expected dangerous characters found
	}{
		{
			name:     "safe_path",
			path:     "/home/user/output.txt",
			expected: []string{},
		},
		{
			name:     "safe_path_with_hyphen_underscore",
			path:     "/home/user/my-file_123.txt",
			expected: []string{},
		},
		{
			name:     "path_with_semicolon",
			path:     "/tmp/file;rm.txt",
			expected: []string{";"},
		},
		{
			name:     "path_with_pipe",
			path:     "/tmp/file|evil.txt",
			expected: []string{"|"},
		},
		{
			name:     "path_with_ampersand",
			path:     "/tmp/file&test.txt",
			expected: []string{"&"},
		},
		{
			name:     "path_with_dollar",
			path:     "/tmp/file$var.txt",
			expected: []string{"$"},
		},
		{
			name:     "path_with_backtick",
			path:     "/tmp/file`cmd`.txt",
			expected: []string{"`"},
		},
		{
			name:     "path_with_redirect",
			path:     "/tmp/file>output.txt",
			expected: []string{">"},
		},
		{
			name:     "path_with_wildcard",
			path:     "/tmp/file*.txt",
			expected: []string{"*"},
		},
		{
			name:     "path_with_question",
			path:     "/tmp/file?.txt",
			expected: []string{"?"},
		},
		{
			name:     "path_with_brackets",
			path:     "/tmp/file[123].txt",
			expected: []string{"[", "]"},
		},
		{
			name:     "path_with_space",
			path:     "/tmp/my file.txt",
			expected: []string{" "},
		},
		{
			name:     "path_with_tab",
			path:     "/tmp/file\ttest.txt",
			expected: []string{"\t"},
		},
		{
			name:     "path_with_newline",
			path:     "/tmp/file\ntest.txt",
			expected: []string{"\n"},
		},
		{
			name:     "path_with_carriage_return",
			path:     "/tmp/file\rtest.txt",
			expected: []string{"\r"},
		},
		{
			name:     "path_with_backslash",
			path:     "/tmp/file\\test.txt",
			expected: []string{"\\"},
		},
		{
			name:     "path_with_multiple_dangerous_chars",
			path:     "/tmp/file;rm|evil*.txt",
			expected: []string{";", "|", "*"},
		},
		{
			name:     "path_with_command_substitution",
			path:     "/tmp/file$(cmd).txt",
			expected: []string{"$("},
		},
		{
			name:     "path_with_variable_expansion",
			path:     "/tmp/file${var}.txt",
			expected: []string{"${"},
		},
		{
			name:     "path_with_logical_and",
			path:     "/tmp/file&&test.txt",
			expected: []string{"&&"},
		},
		{
			name:     "path_with_logical_or",
			path:     "/tmp/file||test.txt",
			expected: []string{"||"},
		},
		{
			name:     "path_with_unicode_space_full_width",
			path:     "/tmp/file　test.txt", // Full-width space (U+3000)
			expected: []string{"　"},
		},
		{
			name:     "path_with_unicode_space_no_break",
			path:     "/tmp/file\u00a0test.txt", // Non-breaking space (U+00A0)
			expected: []string{"\u00a0"},
		},
		{
			name:     "path_with_unicode_space_thin",
			path:     "/tmp/file\u2009test.txt", // Thin space (U+2009)
			expected: []string{"\u2009"},
		},
		{
			name:     "path_with_form_feed",
			path:     "/tmp/file\ftest.txt",
			expected: []string{"\f"},
		},
		{
			name:     "path_with_vertical_tab",
			path:     "/tmp/file\vtest.txt",
			expected: []string{"\v"},
		},
		{
			name:     "path_with_currency_euro",
			path:     "/tmp/file€test.txt",
			expected: []string{"€"},
		},
		{
			name:     "path_with_currency_yen",
			path:     "/tmp/file¥test.txt",
			expected: []string{"¥"},
		},
		{
			name:     "path_with_currency_pound",
			path:     "/tmp/file£test.txt",
			expected: []string{"£"},
		},
		{
			name:     "path_with_currency_rupee",
			path:     "/tmp/file₹test.txt",
			expected: []string{"₹"},
		},
		{
			name:     "path_with_safe_symbols",
			path:     "/tmp/file©®™test.txt", // Copyright, registered, trademark - should be safe
			expected: []string{},
		},
		{
			name:     "path_with_mathematical_symbols",
			path:     "/tmp/file±∞∑test.txt", // Mathematical symbols - should be safe
			expected: []string{},
		},
		{
			name:     "path_with_multiple_unicode_spaces",
			path:     "/tmp/file\u00a0\u2009　test.txt", // Multiple different unicode spaces
			expected: []string{"\u00a0", "\u2009", "　"},
		},
		{
			name:     "path_with_mixed_dangerous_chars_unicode",
			path:     "/tmp/file;€\u00a0*.txt", // Mix of shell metachar, currency, unicode space, glob
			expected: []string{";", "€", "\u00a0", "*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsDangerousCharacters(tt.path)

			if len(tt.expected) == 0 {
				assert.Empty(t, result, "Expected no dangerous characters, but found: %v", result)
			} else {
				assert.NotEmpty(t, result, "Expected dangerous characters but found none")

				// Check that all expected characters are found
				for _, expectedChar := range tt.expected {
					assert.Contains(t, result, expectedChar, "Expected to find dangerous character: %q", expectedChar)
				}
			}
		})
	}
}
