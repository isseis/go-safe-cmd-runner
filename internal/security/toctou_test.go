//go:build test

package security

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
)

// TestCollectTOCTOUCheckDirs verifies that directory collection covers all required
// sources and performs deduplication.
func TestCollectTOCTOUCheckDirs(t *testing.T) {
	tests := []struct {
		name         string
		verifyFiles  []string
		commandPaths []string
		hashDir      string
		wantDirs     []string // sorted expected set (subset check)
		wantNotEmpty bool
	}{
		{
			name:         "empty inputs returns empty",
			verifyFiles:  []string{},
			commandPaths: []string{},
			hashDir:      "",
			wantDirs:     []string{},
		},
		{
			name:         "verify_files parent directories and all ancestors are included",
			verifyFiles:  []string{"/usr/bin/python3", "/etc/hosts"},
			commandPaths: []string{},
			hashDir:      "",
			wantDirs:     []string{"/", "/etc", "/usr", "/usr/bin"},
		},
		{
			name:         "command paths parent directories and all ancestors are included",
			verifyFiles:  []string{},
			commandPaths: []string{"/usr/bin/echo", "/usr/local/bin/tool"},
			hashDir:      "",
			wantDirs:     []string{"/", "/usr", "/usr/bin", "/usr/local", "/usr/local/bin"},
		},
		{
			name:         "hashDir itself is included with ancestors",
			verifyFiles:  []string{},
			commandPaths: []string{},
			hashDir:      "/var/lib/hashes",
			wantDirs:     []string{"/", "/var", "/var/lib", "/var/lib/hashes"},
		},
		{
			name:         "duplicates are removed",
			verifyFiles:  []string{"/usr/bin/python3", "/usr/bin/python2"},
			commandPaths: []string{"/usr/bin/echo"},
			hashDir:      "",
			wantDirs:     []string{"/", "/usr", "/usr/bin"},
		},
		{
			name:         "combined sources without duplicates",
			verifyFiles:  []string{"/usr/bin/python3"},
			commandPaths: []string{"/usr/bin/echo"},
			hashDir:      "/var/hashes",
			wantDirs:     []string{"/", "/usr", "/usr/bin", "/var", "/var/hashes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollectTOCTOUCheckDirs(tt.verifyFiles, tt.commandPaths, tt.hashDir)

			// Sort both slices for comparison
			gotSet := make(map[string]struct{}, len(got))
			for _, d := range got {
				gotSet[d] = struct{}{}
			}

			// Verify all expected dirs are present
			sort.Strings(tt.wantDirs)
			for _, expected := range tt.wantDirs {
				assert.Contains(t, gotSet, expected, "expected directory %q not found in result", expected)
			}

			// Verify no duplicates
			assert.Equal(t, len(got), len(gotSet), "result should contain no duplicate directories")
		})
	}
}

// TestRunTOCTOUPermissionCheck_NoViolations verifies that clean directories
// produce no violations.
func TestRunTOCTOUPermissionCheck_NoViolations(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	err := os.Chmod(tmpDir, 0o755)
	require.NoError(t, err)

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	violations := RunTOCTOUPermissionCheck(v, []string{tmpDir}, slog.Default())
	assert.Empty(t, violations, "no violations expected for secure directory")
}

// TestRunTOCTOUPermissionCheck_ViolationDetected verifies that world-writable
// directories are detected as violations.
func TestRunTOCTOUPermissionCheck_ViolationDetected(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)

	// Make the directory world-writable (violates security policy)
	err := os.Chmod(tmpDir, 0o777)
	require.NoError(t, err)
	// Restore permissions after test so cleanup succeeds
	t.Cleanup(func() {
		_ = os.Chmod(tmpDir, 0o755)
	})

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	violations := RunTOCTOUPermissionCheck(v, []string{tmpDir}, slog.Default())
	require.Len(t, violations, 1, "expected exactly one violation for world-writable directory")
	assert.Equal(t, filepath.Clean(tmpDir), violations[0].Path)
	assert.True(t, errors.Is(violations[0].Err, ErrInvalidDirPermissions), "violation error should be about directory permissions")
}

// TestRunTOCTOUPermissionCheck_MultipleViolations verifies that multiple
// violations are all returned.
func TestRunTOCTOUPermissionCheck_MultipleViolations(t *testing.T) {
	dir1 := commontesting.SafeTempDir(t)
	dir2 := commontesting.SafeTempDir(t)

	for _, d := range []string{dir1, dir2} {
		err := os.Chmod(d, 0o777)
		require.NoError(t, err)
		dd := d
		t.Cleanup(func() {
			_ = os.Chmod(dd, 0o755)
		})
	}

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	violations := RunTOCTOUPermissionCheck(v, []string{dir1, dir2}, slog.Default())
	assert.Len(t, violations, 2, "expected two violations")
}

// TestRunTOCTOUPermissionCheck_EmptyDirs verifies that an empty directory list
// produces no violations.
func TestRunTOCTOUPermissionCheck_EmptyDirs(t *testing.T) {
	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	violations := RunTOCTOUPermissionCheck(v, []string{}, slog.Default())
	assert.Empty(t, violations)
}
