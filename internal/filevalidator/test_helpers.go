//go:build test
// +build test

package filevalidator

import (
	"os"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/stretchr/testify/require"
)

// createTestFile creates a temporary test file with the given content
// and returns the file path. The file is automatically cleaned up when the test ends.
func createTestFile(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp(commontesting.SafeTempDir(t), "test_file_*.txt")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}
