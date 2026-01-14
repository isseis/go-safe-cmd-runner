//go:build test

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPathResolver_ResolvePath(t *testing.T) {
	tests := []struct {
		name         string
		includePath  string
		setupFiles   func(t *testing.T, baseDir string) string // Returns the file path created
		wantErr      bool
		errType      interface{}
		checkAbsPath bool
	}{
		{
			name:        "relative path that exists",
			includePath: "templates/common.toml",
			setupFiles: func(t *testing.T, baseDir string) string {
				err := os.MkdirAll(filepath.Join(baseDir, "templates"), 0o755)
				require.NoError(t, err)
				filePath := filepath.Join(baseDir, "templates", "common.toml")
				err = os.WriteFile(filePath, []byte("test"), 0o644)
				require.NoError(t, err)
				return filePath
			},
			wantErr:      false,
			checkAbsPath: true,
		},
		{
			name:        "relative path with parent directory",
			includePath: "../shared/backup.toml",
			setupFiles: func(t *testing.T, baseDir string) string {
				parentDir := filepath.Dir(baseDir)
				sharedDir := filepath.Join(parentDir, "shared")
				err := os.MkdirAll(sharedDir, 0o755)
				require.NoError(t, err)
				filePath := filepath.Join(sharedDir, "backup.toml")
				err = os.WriteFile(filePath, []byte("test"), 0o644)
				require.NoError(t, err)
				return filePath
			},
			wantErr:      false,
			checkAbsPath: true,
		},
		{
			name:        "file does not exist",
			includePath: "templates/missing.toml",
			setupFiles: func(_ *testing.T, _ string) string {
				return ""
			},
			wantErr: true,
			errType: &ErrIncludedFileNotFound{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			expectedPath := tt.setupFiles(t, baseDir)

			fs := common.NewDefaultFileSystem()
			resolver := NewDefaultPathResolver(fs)
			gotPath, err := resolver.ResolvePath(tt.includePath, baseDir)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.IsType(t, tt.errType, err)
				}
			} else {
				require.NoError(t, err)
				if tt.checkAbsPath {
					// Verify it's an absolute path
					assert.True(t, filepath.IsAbs(gotPath))
					// Verify it matches the expected file
					assert.Equal(t, expectedPath, gotPath)
				}
			}
		})
	}
}

func TestDefaultPathResolver_ResolvePathWithDotPaths(t *testing.T) {
	tests := []struct {
		name        string
		includePath string
		setupFiles  func(t *testing.T, baseDir string) string
	}{
		{
			name:        "path with single dot",
			includePath: "./templates/common.toml",
			setupFiles: func(t *testing.T, baseDir string) string {
				err := os.MkdirAll(filepath.Join(baseDir, "templates"), 0o755)
				require.NoError(t, err)
				filePath := filepath.Join(baseDir, "templates", "common.toml")
				err = os.WriteFile(filePath, []byte("test"), 0o644)
				require.NoError(t, err)
				return filePath
			},
		},
		{
			name:        "path with double dot",
			includePath: "../templates/common.toml",
			setupFiles: func(t *testing.T, baseDir string) string {
				parentDir := filepath.Dir(baseDir)
				err := os.MkdirAll(filepath.Join(parentDir, "templates"), 0o755)
				require.NoError(t, err)
				filePath := filepath.Join(parentDir, "templates", "common.toml")
				err = os.WriteFile(filePath, []byte("test"), 0o644)
				require.NoError(t, err)
				return filePath
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			expectedPath := tt.setupFiles(t, baseDir)

			fs := common.NewDefaultFileSystem()
			resolver := NewDefaultPathResolver(fs)
			gotPath, err := resolver.ResolvePath(tt.includePath, baseDir)

			require.NoError(t, err)
			assert.Equal(t, expectedPath, gotPath)
		})
	}
}

func TestDefaultPathResolver_ErrorDetails(t *testing.T) {
	baseDir := t.TempDir()

	fs := common.NewDefaultFileSystem()
	resolver := NewDefaultPathResolver(fs)

	includePath := "templates/missing.toml"

	_, err := resolver.ResolvePath(includePath, baseDir)

	require.Error(t, err)

	var errNotFound *ErrIncludedFileNotFound
	require.ErrorAs(t, err, &errNotFound)

	assert.Equal(t, includePath, errNotFound.IncludePath)
	assert.Equal(t, baseDir, errNotFound.ReferencedFrom)
	// ResolvedPath should be the cleaned absolute path
	expectedResolved := filepath.Join(baseDir, includePath)
	assert.Equal(t, expectedResolved, errNotFound.ResolvedPath)
}
