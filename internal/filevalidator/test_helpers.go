//go:build test
// +build test

package filevalidator

import (
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/require"
)

// NewTestDynLibValidator creates a Validator configured with the same ELF/Mach-O
// dynamic-library dependency analyzers the `record` command uses. Tests that
// record hashes for real binaries (e.g. /bin/echo) must use this instead of a
// bare New(), otherwise strict dynlib verification fails at run time with
// ErrDynLibDepsRequired.
func NewTestDynLibValidator(t *testing.T, hashDir string) *Validator {
	t.Helper()
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	validator, err := New(&SHA256{}, hashDir, ValidatorConfig{
		ELFDynLibAnalyzer:   elfdynlib.NewDynLibAnalyzer(fs),
		MachODynLibAnalyzer: machodylib.NewMachODynLibAnalyzer(fs),
	})
	require.NoError(t, err)
	return validator
}

// createTestFile creates a temporary test file with the given content
// and returns the file path. The file is automatically cleaned up when the test ends.
func createTestFile(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp(tu.SafeTempDir(t), "test_file_*.txt")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}
