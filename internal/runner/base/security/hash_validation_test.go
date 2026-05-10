package security

import (
	"path/filepath"
	"testing"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func TestValidateFileHash(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)

	t.Run("non-existent file should fail", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non_existent")

		err := validateFileHash(nonExistentFile, tmpDir)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrHashValidationFailed)
	})
}
