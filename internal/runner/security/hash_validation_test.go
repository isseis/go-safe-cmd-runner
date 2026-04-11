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
		config := DefaultConfig()

		err := validateFileHash(nonExistentFile, tmpDir, config)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrHashValidationFailed)
	})

	t.Run("testSkipHashValidation should skip validation", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non_existent")
		config := NewSkipHashValidationTestConfig()

		err := validateFileHash(nonExistentFile, tmpDir, config)
		assert.NoError(t, err, "hash validation should be skipped when testSkipHashValidation is true")
	})

	t.Run("nil config should perform validation", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non_existent")

		err := validateFileHash(nonExistentFile, tmpDir, nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrHashValidationFailed)
	})
}
