//go:build test

package verification

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// invalidHashDirManager returns a Manager configured with a non-existent
// hash directory to exercise hash directory validation failure paths in
// tests.
func invalidHashDirManager() *Manager {
	return &Manager{
		hashDir: "/non/existent/hash/directory",
		fs:      common.NewDefaultFileSystem(),
	}
}

// TestVerifyAndReadConfigFile tests the VerifyAndReadConfigFile method
func TestVerifyAndReadConfigFile(t *testing.T) {
	t.Run("successful_verification_and_read", func(t *testing.T) {
		// Create temporary directory and test config file
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		configContent := `[global]
		max_risk_level = "low"
		[groups.test]
		[[groups.test.commands]]
		command = "echo hello"
		`

		err := os.WriteFile(configPath, []byte(configContent), 0o644)
		require.NoError(t, err)

		// Create manager for testing with hash directory validation skipped
		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Test verification and reading
		content, err := manager.VerifyAndReadConfigFile(configPath)

		// Should succeed when file validator is disabled
		assert.NoError(t, err)
		assert.Equal(t, configContent, string(content))
	})

	t.Run("verification_failure", func(t *testing.T) {
		// Create temporary directory
		tmpDir := t.TempDir()

		// Create manager without disabling file validator (will try to verify hash)
		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		nonExistentConfig := filepath.Join(tmpDir, "nonexistent.toml")

		// Test with non-existent file
		content, err := manager.VerifyAndReadConfigFile(nonExistentConfig)

		// Should fail for non-existent file
		assert.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "verification error")
	})

	t.Run("hash_directory_validation_failure", func(t *testing.T) {
		manager := invalidHashDirManager()

		configPath := "/some/config.toml"

		// Test with invalid hash directory
		content, err := manager.VerifyAndReadConfigFile(configPath)

		// Should fail hash directory validation
		assert.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "ValidateHashDirectory")
	})
}

// TestVerifyEnvironmentFile tests the VerifyEnvironmentFile method
func TestVerifyEnvironmentFile(t *testing.T) {
	t.Run("successful_environment_file_verification", func(t *testing.T) {
		// Create temporary directory and test env file
		tmpDir := t.TempDir()
		envPath := filepath.Join(tmpDir, ".env")
		envContent := `DATABASE_URL=postgresql://localhost/test
		API_KEY=test_key_123
		DEBUG=true
		`

		err := os.WriteFile(envPath, []byte(envContent), 0o600)
		require.NoError(t, err)

		// Create manager for testing with hash directory validation skipped
		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Test verification
		err = manager.VerifyEnvironmentFile(envPath)

		// Should succeed when file validator is disabled
		assert.NoError(t, err)
	})

	t.Run("environment_file_verification_failure", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager without disabling file validator
		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		nonExistentEnv := filepath.Join(tmpDir, "nonexistent.env")

		// Test with non-existent file
		err = manager.VerifyEnvironmentFile(nonExistentEnv)

		// Should fail for non-existent file
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verification error")
	})

	t.Run("hash_directory_validation_failure", func(t *testing.T) {
		manager := invalidHashDirManager()

		envPath := "/some/.env"

		// Test with invalid hash directory
		err := manager.VerifyEnvironmentFile(envPath)

		// Should fail hash directory validation
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ValidateHashDirectory")
	})
}

// TestVerifyGlobalFiles tests the VerifyGlobalFiles method
func TestVerifyGlobalFiles(t *testing.T) {
	t.Run("successful_global_files_verification", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager for testing with hash directory validation skipped
		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create test global config
		globalConfig := &runnertypes.GlobalConfig{
			VerifyFiles: []string{}, // Empty files list should succeed
		}

		// Test verification
		result, err := manager.VerifyGlobalFiles(globalConfig)

		// Should succeed with empty files
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.TotalFiles)
		assert.Equal(t, 0, result.VerifiedFiles)
	})

	t.Run("nil_config_failure", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Test with nil config
		result, err := manager.VerifyGlobalFiles(nil)

		// Should fail with nil config
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, ErrConfigNil))
	})

	t.Run("hash_directory_validation_failure", func(t *testing.T) {
		manager := invalidHashDirManager()

		globalConfig := &runnertypes.GlobalConfig{
			VerifyFiles: []string{},
		}

		// Test with invalid hash directory
		result, err := manager.VerifyGlobalFiles(globalConfig)

		// Should fail hash directory validation
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "ValidateHashDirectory")
	})
}

// TestVerifyGroupFiles tests the VerifyGroupFiles method
func TestVerifyGroupFiles(t *testing.T) {
	t.Run("successful_group_files_verification", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager for testing with hash directory validation skipped
		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create test group config
		groupConfig := &runnertypes.CommandGroup{
			Name:        "test-group",
			VerifyFiles: []string{}, // Empty files list should succeed
		}

		// Test verification
		result, err := manager.VerifyGroupFiles(groupConfig)

		// Should succeed with empty files
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.TotalFiles)
		assert.Equal(t, 0, result.VerifiedFiles)
	})

	t.Run("nil_config_failure", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Test with nil config
		result, err := manager.VerifyGroupFiles(nil)

		// Should fail with nil config
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.True(t, errors.Is(err, ErrConfigNil))
	})

	t.Run("hash_directory_validation_failure", func(t *testing.T) {
		manager := invalidHashDirManager()

		groupConfig := &runnertypes.CommandGroup{
			Name:        "test-group",
			VerifyFiles: []string{},
		}

		// Test with invalid hash directory
		result, err := manager.VerifyGroupFiles(groupConfig)

		// Should fail hash directory validation
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "ValidateHashDirectory")
	})
}

// TestResolvePath tests the ResolvePath method
func TestResolvePath(t *testing.T) {
	t.Run("successful_path_resolution", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager for testing
		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Test with a common command that should exist
		resolvedPath, err := manager.ResolvePath("sh")

		// Should resolve successfully
		assert.NoError(t, err)
		assert.NotEmpty(t, resolvedPath)
		assert.True(t, filepath.IsAbs(resolvedPath))
	})

	t.Run("path_resolver_not_initialized", func(t *testing.T) {
		// Create manager without path resolver
		manager := &Manager{
			hashDir: "/tmp",
			fs:      common.NewDefaultFileSystem(),
			// pathResolver is nil
		}

		// Test path resolution
		resolvedPath, err := manager.ResolvePath("ls")

		// Should fail with path resolver not initialized
		assert.Error(t, err)
		assert.Empty(t, resolvedPath)
		assert.True(t, errors.Is(err, ErrPathResolverNotInitialized))
	})

	t.Run("command_not_found", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Test with non-existent command
		resolvedPath, err := manager.ResolvePath("nonexistent_command_12345")

		// Should fail with command not found
		assert.Error(t, err)
		assert.Empty(t, resolvedPath)
		// The error should contain command not found message
		assert.Contains(t, err.Error(), "command not found")
	})
}

// TestShouldSkipVerification tests the shouldSkipVerification helper method
func TestShouldSkipVerification(t *testing.T) {
	t.Run("skip_verification_conditions", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager for testing with hash directory validation skipped
		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Test path that should be skipped
		shouldSkip := manager.shouldSkipVerification("/tmp/some_file")

		// When file validator is disabled, should skip verification
		assert.True(t, shouldSkip)
	})

	t.Run("do_not_skip_verification", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create manager with file validator enabled
		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Test path that should not be skipped
		shouldSkip := manager.shouldSkipVerification("/usr/bin/ls")

		// When file validator is enabled, should not skip verification
		assert.False(t, shouldSkip)
	})
}

// TestCollectVerificationFiles tests the collectVerificationFiles helper method
func TestCollectVerificationFiles(t *testing.T) {
	t.Run("collect_files_from_config", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Test group config with files
		groupConfig := &runnertypes.CommandGroup{
			Name:        "test-group",
			VerifyFiles: []string{"file1.txt", "file2.txt", "file3.txt"},
		}

		// Collect files
		collectedFiles := manager.collectVerificationFiles(groupConfig)

		// Should return the same files
		assert.Equal(t, groupConfig.VerifyFiles, collectedFiles)
	})

	t.Run("collect_empty_files", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Test group config with empty files
		groupConfig := &runnertypes.CommandGroup{
			Name:        "test-group",
			VerifyFiles: []string{},
		}

		// Collect files
		collectedFiles := manager.collectVerificationFiles(groupConfig)

		// Should return empty slice
		assert.Empty(t, collectedFiles)
	})

	t.Run("collect_nil_group_config", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Collect files with nil input
		collectedFiles := manager.collectVerificationFiles(nil)

		// Should return empty slice
		assert.Empty(t, collectedFiles)
	})
}
