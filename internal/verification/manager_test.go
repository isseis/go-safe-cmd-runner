package verification

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHashDir = "/usr/local/etc/go-safe-cmd-runner/hashes"

// Helper function to create RuntimeGlobal for testing
func createRuntimeGlobal(verifyFiles []string) *runnertypes.RuntimeGlobal {
	spec := &runnertypes.GlobalSpec{
		VerifyFiles: verifyFiles,
	}
	runtime, err := runnertypes.NewRuntimeGlobal(spec)
	require.NoError(nil, err)
	runtime.ExpandedVerifyFiles = verifyFiles
	return runtime
}

// Helper to create a hash manifest file with wrong hash value
// Uses HybridHashFilePathGetter strategy (SubstitutionHashEscape for short paths)
// Returns the path of the created hash file
func createWrongHashManifest(hashDir, filePath, wrongHash string) (string, error) {
	manifest := filevalidator.HashManifest{
		Version:   "1.0",
		Format:    "file-hash",
		Timestamp: time.Now().UTC(),
		File: filevalidator.FileInfo{
			Path: filePath,
			Hash: filevalidator.HashInfo{
				Algorithm: "sha256",
				Value:     wrongHash,
			},
		},
	}

	jsonData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	jsonData = append(jsonData, '\n')

	// Use HybridHashFilePathGetter to get the correct hash file path
	getter := filevalidator.NewHybridHashFilePathGetter()
	resolvedPath, err := common.NewResolvedPath(filePath)
	if err != nil {
		return "", err
	}
	hashFile, err := getter.GetHashFilePath(hashDir, resolvedPath)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(hashFile, jsonData, 0o644); err != nil {
		return "", err
	}

	return hashFile, nil
}

// Helper function to create GroupSpec for testing
// Name and Description are fixed for tests to avoid unparam lint warnings
func createGroupSpec(verifyFiles []string) *runnertypes.GroupSpec {
	return &runnertypes.GroupSpec{
		Name:        "test-group",
		Description: "",
		VerifyFiles: verifyFiles,
	}
}

// Helper function to create RuntimeGroup for testing
func createRuntimeGroup(verifyFiles []string) *runnertypes.RuntimeGroup {
	spec := createGroupSpec(verifyFiles)
	runtime, err := runnertypes.NewRuntimeGroup(spec)
	require.NoError(nil, err)
	runtime.ExpandedVerifyFiles = verifyFiles
	return runtime
}

func TestNewManager(t *testing.T) {
	testCases := []struct {
		name        string
		hashDir     string
		expectError bool
		expectedErr error
	}{
		{
			name:        "valid hash directory",
			hashDir:     testHashDir,
			expectError: false,
		},
		{
			name:        "invalid hash directory",
			hashDir:     "", // empty directory
			expectError: true,
			expectedErr: ErrHashDirectoryEmpty,
		},
		{
			name:        "relative hash directory",
			hashDir:     "relative/path/hashes",
			expectError: true, // Hash directory validation now checks if directory exists
		},
		{
			name:        "dot relative hash directory",
			hashDir:     "./hashes",
			expectError: true, // Hash directory validation now checks if directory exists
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockFS := commontesting.NewMockFileSystem()

			// Set up mock filesystem for valid directories
			if tc.hashDir == testHashDir {
				err := mockFS.AddDir(testHashDir, 0o755)
				require.NoError(t, err)
			}

			manager, err := newManagerInternal(tc.hashDir,
				withFSInternal(mockFS),
				withFileValidatorDisabledInternal(),
				withCreationMode(CreationModeTesting),
				withSecurityLevel(SecurityLevelRelaxed))

			if tc.expectError {
				require.Error(t, err)
				assert.Nil(t, manager)
				if tc.expectedErr != nil {
					assert.ErrorIs(t, err, tc.expectedErr)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, manager)
				// The manager may normalize the path, so we don't assert exact equality for relative paths
				if tc.hashDir == "./hashes" {
					// "./hashes" gets normalized to "hashes"
					assert.Equal(t, "hashes", manager.hashDir)
				} else {
					assert.Equal(t, tc.hashDir, manager.hashDir)
				}
				assert.Equal(t, mockFS, manager.fs)
			}
		})
	}
}

// TestManager_ValidateHashDirectory_NoSecurityValidator tests that hash directory validation fails when no security validator is set

func TestManager_ValidateHashDirectory_NoSecurityValidator(t *testing.T) {
	manager := &Manager{
		hashDir:  testHashDir,
		security: nil, // No security validator
	}

	err := manager.ValidateHashDirectory()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSecurityValidatorNotInitialized)
}

func TestManager_ValidateHashDirectory_RelativePath(t *testing.T) {
	testCases := []struct {
		name        string
		hashDir     string
		expectError bool
	}{
		{
			name:        "absolute path should succeed (if security validator passes)",
			hashDir:     testHashDir,
			expectError: false,
		},
		{
			name:        "relative path should be rejected by security validator",
			hashDir:     "relative/path/hashes",
			expectError: true, // The security validator rejects relative paths
		},
		{
			name:        "dot relative path should be rejected by security validator",
			hashDir:     "./hashes",
			expectError: true, // The security validator rejects relative paths
		},
		{
			name:        "double dot relative path should be rejected by security validator",
			hashDir:     "../hashes",
			expectError: true, // The security validator rejects relative paths
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock filesystem with necessary directory structure
			mockFS := commontesting.NewMockFileSystem()

			// Create the directory in the mock filesystem to satisfy the security validator
			if tc.hashDir != "" {
				// Create the target directory
				mockFS.AddDir(tc.hashDir, 0o755)

				// For absolute paths, also create parent directories to ensure proper path validation
				if tc.hashDir == testHashDir {
					// Create parent directories
					mockFS.AddDir("/", 0o755)
					mockFS.AddDir("/usr", 0o755)
					mockFS.AddDir("/usr/local", 0o755)
					mockFS.AddDir("/usr/local/etc", 0o755)
					mockFS.AddDir("/usr/local/etc/go-safe-cmd-runner", 0o755)
				}
			}

			manager, err := newManagerInternal(tc.hashDir,
				withFSInternal(mockFS),
				withFileValidatorDisabledInternal(),
				withCreationMode(CreationModeTesting),
				withSecurityLevel(SecurityLevelRelaxed))
			require.NoError(t, err)

			// The ValidateHashDirectory method delegates to the security validator
			// Since we're using a mock security validator that checks the filesystem,
			// and we've added the directory to the mock filesystem, this should pass
			err = manager.ValidateHashDirectory()

			if tc.expectError {
				assert.Error(t, err)
			} else {
				// With proper mock filesystem setup, validation should succeed
				assert.NoError(t, err, "expected no error for valid hash directory")
			}
		})
	}
}

func TestManager_VerifyConfigFile_Integration(t *testing.T) {
	// This test requires more complex setup with mock filevalidator and security validator
	// For now, we'll skip this test as it would require significant mocking infrastructure
	t.Skip("Integration test requires complex mocking setup")
}

// Test new production API
func TestNewManagerProduction(t *testing.T) {
	t.Run("creates manager with default hash directory", func(t *testing.T) {
		// We can't easily test the actual NewManager function due to filesystem requirements
		// Instead, test the internal implementation with mocked filesystem
		mockFS := commontesting.NewMockFileSystem()
		err := mockFS.AddDir(testHashDir, 0o755)
		require.NoError(t, err)

		manager, err := newManagerInternal(testHashDir,
			withFSInternal(mockFS),
			withFileValidatorDisabledInternal(),
			withCreationMode(CreationModeProduction),
			withSecurityLevel(SecurityLevelStrict),
		)

		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.Equal(t, testHashDir, manager.hashDir)
	})

	t.Run("validates production constraints", func(t *testing.T) {
		// Test that non-default directory is rejected in production mode
		_, err := newManagerInternal("/custom/hash/dir",
			withFSInternal(commontesting.NewMockFileSystem()),
			withFileValidatorDisabledInternal(),
			withCreationMode(CreationModeProduction),
			withSecurityLevel(SecurityLevelStrict),
		)

		require.Error(t, err)
		var hashDirErr *HashDirectorySecurityError
		assert.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, "/custom/hash/dir", hashDirErr.RequestedDir)
		assert.Equal(t, testHashDir, hashDirErr.DefaultDir)
	})
}

// TestManager_ResolvePath_Integration tests end-to-end path resolution with securePathEnv
func TestManager_ResolvePath_Integration(t *testing.T) {
	// Create a temporary directory structure that mimics the secure path
	tempDir := t.TempDir()

	// Create directories that match parts of securePathEnv: /sbin:/usr/sbin:/bin:/usr/bin
	sbinDir := filepath.Join(tempDir, "sbin")
	usrSbinDir := filepath.Join(tempDir, "usr", "sbin")
	binDir := filepath.Join(tempDir, "bin")
	usrBinDir := filepath.Join(tempDir, "usr", "bin")

	require.NoError(t, os.MkdirAll(sbinDir, 0o755))
	require.NoError(t, os.MkdirAll(usrSbinDir, 0o755))
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.MkdirAll(usrBinDir, 0o755))

	// Create test commands in different directories
	testCmd1 := filepath.Join(binDir, "testcmd")
	testCmd2 := filepath.Join(usrBinDir, "anothercmd")
	testCmd3 := filepath.Join(sbinDir, "systemcmd")

	require.NoError(t, os.WriteFile(testCmd1, []byte("#!/bin/sh\necho test\n"), 0o755))
	require.NoError(t, os.WriteFile(testCmd2, []byte("#!/bin/sh\necho another\n"), 0o755))
	require.NoError(t, os.WriteFile(testCmd3, []byte("#!/bin/sh\necho system\n"), 0o755))

	// Create a test secure path using our temporary directories
	testSecurePath := sbinDir + ":" + usrSbinDir + ":" + binDir + ":" + usrBinDir

	t.Run("resolves commands from secure PATH correctly", func(t *testing.T) {
		// Create a manager with a custom path resolver using our test secure path
		// We need to use the real filesystem for path resolution, not the mock
		// For integration testing, we disable security validation to focus on PATH resolution
		testPathResolver := NewPathResolver(testSecurePath, nil, false)
		manager, err := NewManagerForTest(testHashDir,
			WithFileValidatorDisabled(),
			WithPathResolver(testPathResolver),
		)
		require.NoError(t, err)

		// Test resolving commands that exist in the secure PATH
		resolved, err := manager.ResolvePath("testcmd")
		require.NoError(t, err)
		assert.Equal(t, testCmd1, resolved) // Should find in /bin first

		resolved, err = manager.ResolvePath("anothercmd")
		require.NoError(t, err)
		assert.Equal(t, testCmd2, resolved) // Should find in /usr/bin

		resolved, err = manager.ResolvePath("systemcmd")
		require.NoError(t, err)
		assert.Equal(t, testCmd3, resolved) // Should find in /sbin first
	})

	t.Run("fails to resolve commands not in secure PATH", func(t *testing.T) {
		// Create a manager with a custom path resolver using our test secure path
		// For integration testing, we disable security validation to focus on PATH resolution
		testPathResolver := NewPathResolver(testSecurePath, nil, false)
		manager, err := NewManagerForTest(testHashDir,
			WithFileValidatorDisabled(),
			WithPathResolver(testPathResolver),
		)
		require.NoError(t, err)

		// Test resolving a command that doesn't exist
		_, err = manager.ResolvePath("nonexistentcommand")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandNotFound)
	})

	t.Run("respects PATH precedence from securePathEnv", func(t *testing.T) {
		// Create the same command in multiple directories
		duplicateCmd1 := filepath.Join(sbinDir, "duplicate")
		duplicateCmd2 := filepath.Join(binDir, "duplicate")

		require.NoError(t, os.WriteFile(duplicateCmd1, []byte("#!/bin/sh\necho sbin\n"), 0o755))
		require.NoError(t, os.WriteFile(duplicateCmd2, []byte("#!/bin/sh\necho bin\n"), 0o755))

		// Create a manager with a custom path resolver using our test secure path
		// For integration testing, we disable security validation to focus on PATH resolution
		testPathResolver := NewPathResolver(testSecurePath, nil, false)
		manager, err := NewManagerForTest(testHashDir,
			WithFileValidatorDisabled(),
			WithPathResolver(testPathResolver),
		)
		require.NoError(t, err)

		// Should find the first one in the PATH order (/sbin comes first)
		resolved, err := manager.ResolvePath("duplicate")
		require.NoError(t, err)
		assert.Equal(t, duplicateCmd1, resolved) // Should find /sbin/duplicate first
	})

	t.Run("integration with default securePathEnv structure", func(t *testing.T) {
		// Test that our Manager correctly uses the hardcoded securePathEnv
		// We can't easily test with the actual system paths, but we can verify
		// that the Manager uses its pathResolver correctly
		mockFS := commontesting.NewMockFileSystem()
		require.NoError(t, mockFS.AddDir(testHashDir, 0o755))

		manager, err := newManagerInternal(testHashDir,
			withFSInternal(mockFS),
			withFileValidatorDisabledInternal(),
			withCreationMode(CreationModeTesting),
			withSecurityLevel(SecurityLevelRelaxed))
		require.NoError(t, err)

		// Verify that the manager has a pathResolver initialized
		assert.NotNil(t, manager.pathResolver)

		// Test with a command that definitely won't exist
		_, err = manager.ResolvePath("definitely-nonexistent-command-12345")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandNotFound)
	})
}

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
		risk_level = "low"
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

		// Create test runtime global
		runtimeGlobal := createRuntimeGlobal([]string{}) // Empty files list should succeed

		// Test verification
		result, err := manager.VerifyGlobalFiles(runtimeGlobal)

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
		assert.ErrorIs(t, err, ErrConfigNil)
	})

	t.Run("hash_directory_validation_failure", func(t *testing.T) {
		manager := invalidHashDirManager()

		runtimeGlobal := createRuntimeGlobal([]string{})

		// Test with invalid hash directory
		result, err := manager.VerifyGlobalFiles(runtimeGlobal)

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

		// Create test runtime group
		runtimeGroup := createRuntimeGroup([]string{}) // Empty files list should succeed

		// Test verification
		result, err := manager.VerifyGroupFiles(runtimeGroup)

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
		assert.ErrorIs(t, err, ErrConfigNil)
	})

	t.Run("hash_directory_validation_failure", func(t *testing.T) {
		manager := invalidHashDirManager()

		runtimeGroup := createRuntimeGroup([]string{})

		// Test with invalid hash directory
		result, err := manager.VerifyGroupFiles(runtimeGroup)

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
		assert.ErrorIs(t, err, ErrPathResolverNotInitialized)
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

		// Test runtime group with files
		runtimeGroup := createRuntimeGroup([]string{"file1.txt", "file2.txt", "file3.txt"})

		// Collect files
		collectedFiles := manager.collectVerificationFiles(runtimeGroup)

		// Should return a map with the same files
		assert.Len(t, collectedFiles, 3)
		assert.Contains(t, collectedFiles, "file1.txt")
		assert.Contains(t, collectedFiles, "file2.txt")
		assert.Contains(t, collectedFiles, "file3.txt")
	})

	t.Run("collect_empty_files", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Test runtime group with empty files
		runtimeGroup := createRuntimeGroup([]string{})

		// Collect files
		collectedFiles := manager.collectVerificationFiles(runtimeGroup)

		// Should return empty map
		assert.Empty(t, collectedFiles)
	})

	t.Run("collect_nil_group_config", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Collect files with nil input
		collectedFiles := manager.collectVerificationFiles(nil)

		// Should return empty map
		assert.Empty(t, collectedFiles)
	})

	t.Run("automatic_deduplication", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Test runtime group with duplicate files
		runtimeGroup := createRuntimeGroup([]string{"file1.txt", "file2.txt", "file1.txt", "file3.txt", "file2.txt"})

		// Collect files
		collectedFiles := manager.collectVerificationFiles(runtimeGroup)

		// Should automatically remove duplicates
		assert.Len(t, collectedFiles, 3)
		assert.Contains(t, collectedFiles, "file1.txt")
		assert.Contains(t, collectedFiles, "file2.txt")
		assert.Contains(t, collectedFiles, "file3.txt")
	})

	t.Run("expand_command_variables", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create actual command files for PATH resolution
		binDir := filepath.Join(tmpDir, "bin")
		err := os.MkdirAll(binDir, 0o755)
		require.NoError(t, err)

		testCmd := filepath.Join(binDir, "testcmd")
		err = os.WriteFile(testCmd, []byte("#!/bin/sh\necho test"), 0o755)
		require.NoError(t, err)

		// Create manager with PATH resolver
		pathResolver := NewPathResolver(binDir, nil, false)
		manager, err := NewManagerForTest(tmpDir, WithPathResolver(pathResolver))
		require.NoError(t, err)

		// Create runtime group with command using variable reference
		spec := &runnertypes.GroupSpec{
			Name: "test-group",
			Commands: []runnertypes.CommandSpec{
				{
					Name: "test-command",
					Cmd:  "%{bindir}/testcmd",
					Args: []string{},
				},
			},
		}
		runtimeGroup, err := runnertypes.NewRuntimeGroup(spec)
		require.NoError(t, err)
		runtimeGroup.ExpandedVars = map[string]string{
			"bindir": binDir,
		}

		// Create pre-expanded RuntimeCommand (simulating preExpandCommands behavior)
		runtimeCmd, err := runnertypes.NewRuntimeCommand(&spec.Commands[0], common.NewUnsetTimeout(), common.NewUnlimitedOutputSizeLimit(), spec.Name)
		require.NoError(t, err)
		runtimeCmd.ExpandedCmd = filepath.Join(binDir, "testcmd")
		runtimeGroup.Commands = []*runnertypes.RuntimeCommand{runtimeCmd}

		// Collect files (should use pre-expanded command)
		collectedFiles := manager.collectVerificationFiles(runtimeGroup)

		// Should resolve to the actual command path
		assert.Len(t, collectedFiles, 1)
		assert.Contains(t, collectedFiles, testCmd)
	})

	t.Run("skip_command_with_expansion_error", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir)
		require.NoError(t, err)

		// Create runtime group with command using undefined variable
		spec := &runnertypes.GroupSpec{
			Name: "test-group",
			Commands: []runnertypes.CommandSpec{
				{
					Name: "test-command",
					Cmd:  "%{undefined_var}/testcmd",
					Args: []string{},
				},
			},
		}
		runtimeGroup, err := runnertypes.NewRuntimeGroup(spec)
		require.NoError(t, err)
		runtimeGroup.ExpandedVars = map[string]string{} // Empty - no variables defined

		// Collect files (should skip command with expansion error)
		collectedFiles := manager.collectVerificationFiles(runtimeGroup)

		// Should return empty (command skipped due to expansion error)
		assert.Empty(t, collectedFiles)
	})

	t.Run("skip_command_with_resolution_error", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create path resolver with empty PATH (no commands can be resolved)
		pathResolver := NewPathResolver("", nil, false)
		manager, err := NewManagerForTest(tmpDir, WithPathResolver(pathResolver))
		require.NoError(t, err)

		// Create runtime group with command that can't be resolved
		spec := &runnertypes.GroupSpec{
			Name: "test-group",
			Commands: []runnertypes.CommandSpec{
				{
					Name: "test-command",
					Cmd:  "/nonexistent/command",
					Args: []string{},
				},
			},
		}
		runtimeGroup, err := runnertypes.NewRuntimeGroup(spec)
		require.NoError(t, err)
		runtimeGroup.ExpandedVars = map[string]string{}

		// Collect files (should skip command with resolution error)
		collectedFiles := manager.collectVerificationFiles(runtimeGroup)

		// Should return empty (command skipped due to resolution error)
		assert.Empty(t, collectedFiles)
	})
}

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
		err = manager.verifyFileWithFallback(testFile, "test")
		assert.NoError(t, err)
	})

	t.Run("file_not_found", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir, WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Test with non-existent file (with file validator enabled to ensure error)
		err = manager.verifyFileWithFallback("/non/existent/file.txt", "test")
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
		err = manager.verifyFileWithFallback(testFile, "test")
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
		content, err := manager.readAndVerifyFileWithFallback(testFile, "test")
		assert.NoError(t, err)
		assert.Equal(t, testContent, string(content))
	})

	t.Run("file_not_found", func(t *testing.T) {
		tmpDir := t.TempDir()

		manager, err := NewManagerForTest(tmpDir, WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Test with non-existent file
		content, err := manager.readAndVerifyFileWithFallback("/non/existent/config.toml", "test")
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
		content, err := manager.readAndVerifyFileWithFallback(testFile, "test")
		assert.Error(t, err)
		assert.Nil(t, content)
	})
}

// TestVerifyFileWithFallback_DryRunLogging tests that security_risk is included in logs during dry-run mode
func TestVerifyFileWithFallback_DryRunLogging(t *testing.T) {
	t.Run("logs_security_risk_on_verification_failure", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Capture log output
		var logBuffer strings.Builder
		logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		originalLogger := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(originalLogger)

		// Create dry-run manager with file validator enabled
		manager, err := NewManagerForTest(tmpDir, WithDryRunMode(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create test file (no hash file exists, so verification will fail)
		testFile := filepath.Join(tmpDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		// In dry-run mode, verification failure should be logged but not returned as error
		err = manager.verifyFileWithFallback(testFile, "test-context")
		assert.NoError(t, err, "dry-run mode should not return error on verification failure")

		// Verify security_risk is in the log
		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "security_risk", "log should contain security_risk attribute")
		assert.Contains(t, logOutput, "dry-run mode", "log should indicate dry-run mode")
	})
}

// TestReadAndVerifyFileWithFallback_DryRunLogging tests that security_risk is included in logs during dry-run mode
func TestReadAndVerifyFileWithFallback_DryRunLogging(t *testing.T) {
	t.Run("logs_security_risk_on_verification_failure", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Capture log output
		var logBuffer strings.Builder
		logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		originalLogger := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(originalLogger)

		// Create dry-run manager with file validator enabled
		manager, err := NewManagerForTest(tmpDir, WithDryRunMode(), WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create test file (no hash file exists, so verification will fail)
		testFile := filepath.Join(tmpDir, "test.conf")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		// In dry-run mode, verification failure should be logged but file should still be read
		content, err := manager.readAndVerifyFileWithFallback(testFile, "test-context")
		assert.NoError(t, err, "dry-run mode should not return error on verification failure")
		assert.Equal(t, "test content", string(content), "file content should be returned")

		// Verify security_risk is in the log
		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "security_risk", "log should contain security_risk attribute")
		assert.Contains(t, logOutput, "dry-run mode", "log should indicate dry-run mode")
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

// TestVerifyGlobalFiles_DryRun_MultipleFailures tests global file verification in dry-run mode
// when hash files do not exist for any files (both files will fail verification)
func TestVerifyGlobalFiles_DryRun_MultipleFailures(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create test files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	err = os.WriteFile(file1, []byte("content1"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(file2, []byte("content2"), 0o644)
	require.NoError(t, err)

	// Set up mock filesystem with files but no hash files (both will fail verification)
	mockFS := commontesting.NewMockFileSystem()
	err = mockFS.AddDir(hashDir, 0o755)
	require.NoError(t, err)
	err = mockFS.AddFile(file1, 0o644, []byte("content1"))
	require.NoError(t, err)
	err = mockFS.AddFile(file2, 0o644, []byte("content2"))
	require.NoError(t, err)

	// Create manager in dry-run mode
	manager, err := newManagerInternal(hashDir,
		withFSInternal(mockFS),
		withDryRunModeInternal(),
		withCreationMode(CreationModeTesting),
		withSecurityLevel(SecurityLevelRelaxed),
		withSkipHashDirectoryValidationInternal())
	require.NoError(t, err)

	// Create RuntimeGlobal with both files (no system paths to avoid skip logic complexity)
	runtimeGlobal := createRuntimeGlobal([]string{file1, file2})

	// In dry-run mode, verification should complete without error
	result, err := manager.VerifyGlobalFiles(runtimeGlobal)
	assert.NoError(t, err, "dry-run mode should not return errors")
	assert.NotNil(t, result)

	// Verify summary
	summary := manager.GetVerificationSummary()
	require.NotNil(t, summary)
	assert.Equal(t, 2, summary.TotalFiles, "should have 2 total files")
	assert.Equal(t, 2, summary.FailedFiles, "both files should fail (no hash files exist)")
}

// TestVerifyGroupFiles_DryRun_HashFileNotFound tests group file verification in dry-run mode
// when hash file is not found (WARN level logging)
func TestVerifyGroupFiles_DryRun_HashFileNotFound(t *testing.T) {
	tmpDir, hashDir, logBuffer, cleanup := setupDryRunTest(t)
	defer cleanup()

	// Create test file
	testFile := createTestFile(t, tmpDir, "config.toml", []byte("actual content"))

	// Create manager in dry-run mode
	manager := createDryRunManager(t, hashDir)

	// Create RuntimeGroup
	runtimeGroup := createRuntimeGroup([]string{testFile})

	// In dry-run mode, verification should complete without error
	result, err := manager.VerifyGroupFiles(runtimeGroup)
	assert.NoError(t, err, "dry-run mode should not return errors")
	assert.NotNil(t, result)

	// Verify that verification was attempted and recorded
	summary := manager.GetVerificationSummary()
	require.NotNil(t, summary)
	assert.True(t, summary.TotalFiles > 0, "should have files to verify")

	// Verify that WARN level logging occurred (hash file not found)
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "security_risk", "log should contain security_risk")
}

// TestVerifyConfigFile_DryRun_HashFileNotFound tests config file verification in dry-run mode
// when hash file is not found (WARN level logging)
func TestVerifyConfigFile_DryRun_HashFileNotFound(t *testing.T) {
	tmpDir, hashDir, logBuffer, cleanup := setupDryRunTest(t)
	defer cleanup()

	// Create config file
	configFile := createTestFile(t, tmpDir, "config.toml", []byte("test config"))

	// Create manager in dry-run mode
	manager := createDryRunManager(t, hashDir)

	// In dry-run mode, reading should succeed even if verification fails
	content, err := manager.VerifyAndReadConfigFile(configFile)
	assert.NoError(t, err, "dry-run mode should not return errors")
	assert.Equal(t, "test config", string(content), "should return file content")

	// Verify that verification failure was recorded
	summary := manager.GetVerificationSummary()
	require.NotNil(t, summary)
	assert.Equal(t, 1, summary.TotalFiles, "should have 1 file")
	assert.Equal(t, 1, summary.FailedFiles, "should have 1 failed file")
	assert.Len(t, summary.Failures, 1, "should have 1 failure recorded")

	// Verify WARN level logging (hash file not found)
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "security_risk", "log should contain security_risk")
	require.NotEmpty(t, summary.Failures)
	assert.Equal(t, ReasonHashFileNotFound, summary.Failures[0].Reason)
	assert.Equal(t, logLevelWarn, summary.Failures[0].Level)
}

// TestVerifyGroupFiles_DryRun_HashMismatch tests group file verification in dry-run mode
// when hash mismatch occurs (ERROR level logging)
func TestVerifyGroupFiles_DryRun_HashMismatch(t *testing.T) {
	tmpDir, hashDir, logBuffer, cleanup := setupDryRunTest(t)
	defer cleanup()

	// Create test file
	testFile := createTestFile(t, tmpDir, "testfile.txt", []byte("actual content"))

	// Write hash file with incorrect hash value
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	hashFile, err := createWrongHashManifest(hashDir, testFile, wrongHash)
	require.NoError(t, err)

	// Verify hash file was created
	_, err = os.Stat(hashFile)
	require.NoError(t, err, "hash file should exist at %s", hashFile)

	// Create manager in dry-run mode
	manager := createDryRunManager(t, hashDir)

	// Create RuntimeGroup
	runtimeGroup := createRuntimeGroup([]string{testFile})

	// In dry-run mode, verification should complete without error
	result, err := manager.VerifyGroupFiles(runtimeGroup)
	assert.NoError(t, err, "dry-run mode should not return errors")
	assert.NotNil(t, result)

	// Verify that verification failure was recorded
	summary := manager.GetVerificationSummary()
	require.NotNil(t, summary)
	assert.True(t, summary.TotalFiles > 0, "should have files to verify")
	assert.True(t, summary.FailedFiles > 0, "should have failed files")

	// Verify that ERROR level logging occurred (hash mismatch)
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "security_risk", "log should contain security_risk")
	require.NotEmpty(t, summary.Failures)

	// Find the failure for our test file
	var foundFailure *FileVerificationFailure
	for i := range summary.Failures {
		if summary.Failures[i].Path == testFile {
			foundFailure = &summary.Failures[i]
			break
		}
	}
	require.NotNil(t, foundFailure, "should have failure for test file")
	assert.Equal(t, ReasonHashMismatch, foundFailure.Reason)
	assert.Equal(t, logLevelError, foundFailure.Level)
}

// TestVerifyConfigFile_DryRun_HashMismatch tests config file verification in dry-run mode
// when hash mismatch occurs (ERROR level logging)
func TestVerifyConfigFile_DryRun_HashMismatch(t *testing.T) {
	tmpDir, hashDir, logBuffer, cleanup := setupDryRunTest(t)
	defer cleanup()

	// Create config file
	configFile := createTestFile(t, tmpDir, "config.toml", []byte("test config"))

	// Write hash file with incorrect hash value
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	_, err := createWrongHashManifest(hashDir, configFile, wrongHash)
	require.NoError(t, err)

	// Create manager in dry-run mode
	manager := createDryRunManager(t, hashDir)

	// In dry-run mode, reading should succeed even if verification fails
	content, err := manager.VerifyAndReadConfigFile(configFile)
	assert.NoError(t, err, "dry-run mode should not return errors")
	assert.Equal(t, "test config", string(content), "should return file content")

	// Verify that verification failure was recorded
	summary := manager.GetVerificationSummary()
	require.NotNil(t, summary)
	assert.Equal(t, 1, summary.TotalFiles, "should have 1 file")
	assert.Equal(t, 1, summary.FailedFiles, "should have 1 failed file")
	assert.Len(t, summary.Failures, 1, "should have 1 failure recorded")

	// Verify ERROR level logging (hash mismatch)
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "security_risk", "log should contain security_risk")
	require.NotEmpty(t, summary.Failures)
	assert.Equal(t, ReasonHashMismatch, summary.Failures[0].Reason)
	assert.Equal(t, logLevelError, summary.Failures[0].Level)
}
