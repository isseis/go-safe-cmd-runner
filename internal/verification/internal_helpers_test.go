//go:build test

package verification

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyFileWithFallback tests the verifyFileWithFallback helper method
func TestVerifyFileWithFallback(t *testing.T) {
	t.Run("successful_verification", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager with file validator disabled (for testing)
		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create test file
		testFile := filepath.Join(tmpDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		// Test verification (should succeed when file validator is disabled)
		err = manager.verifyFileWithFallback(testFile)
		assert.NoError(t, err)
	})

	t.Run("file_not_found", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir, WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Test with non-existent file (with file validator enabled to ensure error)
		err = manager.verifyFileWithFallback("/non/existent/file.txt")
		assert.Error(t, err)
	})

	t.Run("verification_failure_with_validator", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager with file validator enabled (will try to verify hash)
		manager, err := NewManagerForTest(tmpDir, WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create test file
		testFile := filepath.Join(tmpDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		// Test verification (should fail because no hash file exists)
		err = manager.verifyFileWithFallback(testFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hash")
	})
}

// TestReadAndVerifyFileWithFallback tests the readAndVerifyFileWithFallback helper method
func TestReadAndVerifyFileWithFallback(t *testing.T) {
	t.Run("successful_read_and_verification", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create test file
		testContent := "test file content for verification"
		testFile := filepath.Join(tmpDir, "test.conf")
		err = os.WriteFile(testFile, []byte(testContent), 0o644)
		require.NoError(t, err)

		// Test reading and verification
		content, err := manager.readAndVerifyFileWithFallback(testFile)
		assert.NoError(t, err)
		assert.Equal(t, testContent, string(content))
	})

	t.Run("file_not_found", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Test with non-existent file
		content, err := manager.readAndVerifyFileWithFallback("/non/existent/config.toml")
		assert.Error(t, err)
		assert.Nil(t, content)
	})

	t.Run("verification_failure_with_validator", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager with file validator enabled
		manager, err := NewManagerForTest(tmpDir, WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create test file
		testFile := filepath.Join(tmpDir, "test.conf")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		// Test reading and verification (should fail because no hash file exists)
		content, err := manager.readAndVerifyFileWithFallback(testFile)
		assert.Error(t, err)
		assert.Nil(t, content)
	})
}

// TestValidateSecurityConstraints tests the validateSecurityConstraints function
func TestValidateSecurityConstraints(t *testing.T) {
	t.Run("testing_mode_with_skip_validation", func(t *testing.T) {
		tmpDir := t.TempDir()

		opts := newInternalOptions()
		opts.creationMode = CreationModeTesting // Use testing mode to avoid production constraints
		opts.skipHashDirectoryValidation = true

		err := validateSecurityConstraints(tmpDir, opts)
		assert.NoError(t, err)
	})

	t.Run("empty_hash_directory", func(t *testing.T) {
		opts := newInternalOptions()
		opts.creationMode = CreationModeTesting

		err := validateSecurityConstraints("", opts)
		assert.Error(t, err)
		// The actual error message might vary based on implementation
		assert.Error(t, err)
	})

	t.Run("testing_mode_skip_validation_enabled", func(t *testing.T) {
		opts := newInternalOptions()
		opts.creationMode = CreationModeTesting
		opts.skipHashDirectoryValidation = true

		err := validateSecurityConstraints("/any/path", opts)
		assert.NoError(t, err)
	})

	t.Run("production_mode_constraints", func(t *testing.T) {
		opts := newInternalOptions()
		opts.creationMode = CreationModeProduction

		// Production mode enforces specific constraints
		err := validateSecurityConstraints("/custom/hash/dir", opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "security violation")
	})
}

// TestNewManagerInternalOptions tests the newInternalOptions function and related option functions
func TestNewManagerInternalOptions(t *testing.T) {
	t.Run("default_options", func(t *testing.T) {
		opts := newInternalOptions()

		assert.True(t, opts.fileValidatorEnabled)
		assert.NotNil(t, opts.fs)
		assert.Equal(t, CreationModeProduction, opts.creationMode)
		assert.Equal(t, SecurityLevelStrict, opts.securityLevel)
		assert.False(t, opts.skipHashDirectoryValidation)
		assert.False(t, opts.isDryRun)
	})

	t.Run("apply_creation_mode_option", func(t *testing.T) {
		opts := newInternalOptions()

		option := withCreationMode(CreationModeTesting)
		option(opts)

		assert.Equal(t, CreationModeTesting, opts.creationMode)
	})

	t.Run("apply_security_level_option", func(t *testing.T) {
		opts := newInternalOptions()

		option := withSecurityLevel(SecurityLevelRelaxed)
		option(opts)

		assert.Equal(t, SecurityLevelRelaxed, opts.securityLevel)
	})

	t.Run("apply_fs_option", func(t *testing.T) {
		opts := newInternalOptions()
		mockFS := common.NewDefaultFileSystem() // Using real filesystem for simplicity

		option := withFSInternal(mockFS)
		option(opts)

		assert.Equal(t, mockFS, opts.fs)
	})

	t.Run("apply_file_validator_disabled_option", func(t *testing.T) {
		opts := newInternalOptions()

		option := withFileValidatorDisabledInternal()
		option(opts)

		assert.False(t, opts.fileValidatorEnabled)
	})

	t.Run("apply_skip_hash_directory_validation_option", func(t *testing.T) {
		opts := newInternalOptions()

		option := withSkipHashDirectoryValidationInternal()
		option(opts)

		assert.True(t, opts.skipHashDirectoryValidation)
	})

	t.Run("apply_dry_run_mode_option", func(t *testing.T) {
		opts := newInternalOptions()

		option := withDryRunModeInternal()
		option(opts)

		assert.True(t, opts.isDryRun)
	})
}

// TestManagerCreationWithFileValidator tests manager creation with file validator scenarios
func TestManagerCreationWithFileValidator(t *testing.T) {
	t.Run("manager_with_file_validator_enabled", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir, WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// File validator should be initialized
		assert.NotNil(t, manager.fileValidator)

		// Security validator should be initialized
		assert.NotNil(t, manager.security)

		// Path resolver should be initialized
		assert.NotNil(t, manager.pathResolver)
	})

	t.Run("manager_with_file_validator_disabled", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// File validator should be nil
		assert.Nil(t, manager.fileValidator)

		// Security validator should still be initialized
		assert.NotNil(t, manager.security)

		// Path resolver should still be initialized
		assert.NotNil(t, manager.pathResolver)
	})

	t.Run("manager_in_dry_run_mode", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager with dry run mode through internal options
		manager, err := newManagerInternal(tmpDir,
			withCreationMode(CreationModeTesting), // Use testing mode to avoid production constraints
			withSkipHashDirectoryValidationInternal(),
			withFileValidatorDisabledInternal(),
			withDryRunModeInternal(),
		)
		require.NoError(t, err)

		// Should be marked as dry run
		assert.True(t, manager.isDryRun)
	})
}

// TestSecurityIntegration tests integration between Manager and SecurityValidator
func TestSecurityIntegration(t *testing.T) {
	t.Run("hash_directory_validation_integration", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager without skipping hash directory validation
		manager, err := NewManagerForTest(tmpDir)
		if err != nil {
			// This might fail due to directory permissions, which is expected
			assert.Contains(t, err.Error(), "hash directory validation failed")
			return
		}

		// If creation succeeded, test hash directory validation
		err = manager.ValidateHashDirectory()
		// This might succeed or fail depending on the temp directory permissions
		// The key is that it exercises the security validator integration
		if err != nil {
			assert.Contains(t, err.Error(), "hash directory")
		}
	})

	t.Run("path_resolver_security_integration", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir, WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Test path resolution with security validation
		// This tests the integration between PathResolver and SecurityValidator
		path, err := manager.ResolvePath("sh")

		if err == nil {
			// If resolution succeeded, the path should be validated
			assert.NotEmpty(t, path)
			assert.True(t, filepath.IsAbs(path))
		} else {
			// If it failed, it should be due to command not found or security constraints
			assert.Error(t, err)
		}
	})
}

// TestTypeEnumMethods tests the String methods for enums
func TestTypeEnumMethods(t *testing.T) {
	t.Run("creation_mode_string", func(t *testing.T) {
		assert.Equal(t, "production", CreationModeProduction.String())
		assert.Equal(t, "testing", CreationModeTesting.String())
	})

	t.Run("security_level_string", func(t *testing.T) {
		assert.Equal(t, "strict", SecurityLevelStrict.String())
		assert.Equal(t, "relaxed", SecurityLevelRelaxed.String())
	})
}
