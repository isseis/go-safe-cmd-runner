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
		{
			name:        "whitespace_only_path",
			outputPath:  "   ",
			workDir:     "/home/user",
			wantErr:     true,
			errContains: "output path is empty",
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
			name:        "path_with_double_dot",
			path:        "/tmp/../etc/passwd",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:        "path_with_double_dot_at_end",
			path:        "/tmp/..",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:        "path_with_double_dot_at_start",
			path:        "/../tmp/file",
			wantErr:     true,
			errContains: "path traversal detected",
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
			name:        "relative_path_with_double_dot",
			path:        "../output.txt",
			workDir:     "/home/user/project",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:        "relative_path_escapes_workdir",
			path:        "../../../../etc/passwd",
			workDir:     "/home/user/project",
			wantErr:     true,
			errContains: "path traversal detected",
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
