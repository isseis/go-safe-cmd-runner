package security

import (
	"path/filepath"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/stretchr/testify/assert"
)

func TestValidateFileHash(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)

	t.Run("non-existent file should fail", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non_existent")

		err := validateFileHash(nonExistentFile, tmpDir)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrHashValidationFailed)
	})
}
