package verification

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHashDir = "/usr/local/etc/go-safe-cmd-runner/hashes"

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
			mockFS := common.NewMockFileSystem()

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
			mockFS := common.NewMockFileSystem()

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

// Test error wrapping in VerifyConfigFile
func TestManager_VerifyConfigFile_ErrorWrapping(t *testing.T) {
	// Create manager with mocked components that will fail
	mockFS := common.NewMockFileSystem()
	manager := &Manager{
		hashDir: testHashDir,
		fs:      mockFS,
		// Leave validator and security nil to trigger errors
	}

	err := manager.VerifyConfigFile("/path/to/config.toml")
	assert.Error(t, err)

	// Check that error is properly wrapped
	var verificationErr *Error
	assert.True(t, errors.As(err, &verificationErr))
	assert.Equal(t, "ValidateHashDirectory", verificationErr.Op)
}

// Test new production API
func TestNewManagerProduction(t *testing.T) {
	t.Run("creates manager with default hash directory", func(t *testing.T) {
		// We can't easily test the actual NewManager function due to filesystem requirements
		// Instead, test the internal implementation with mocked filesystem
		mockFS := common.NewMockFileSystem()
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
			withFSInternal(common.NewMockFileSystem()),
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
		// Create a manager with a custom securePathEnv for testing
		mockFS := common.NewMockFileSystem()
		require.NoError(t, mockFS.AddDir(testHashDir, 0o755))

		// Create manager using internal constructor with custom options
		manager, err := newManagerInternal(testHashDir,
			withFSInternal(mockFS),
			withFileValidatorDisabledInternal(),
			withCreationMode(CreationModeTesting),
			withSecurityLevel(SecurityLevelRelaxed))
		require.NoError(t, err)

		// Override the pathResolver to use our test secure path
		// We need to use the real filesystem for path resolution, not the mock
		// For integration testing, we disable security validation to focus on PATH resolution
		manager.pathResolver = NewPathResolver(testSecurePath, nil, false)

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
		// Create a manager with our test secure path
		mockFS := common.NewMockFileSystem()
		require.NoError(t, mockFS.AddDir(testHashDir, 0o755))

		manager, err := newManagerInternal(testHashDir,
			withFSInternal(mockFS),
			withFileValidatorDisabledInternal(),
			withCreationMode(CreationModeTesting),
			withSecurityLevel(SecurityLevelRelaxed))
		require.NoError(t, err)

		// Override the pathResolver to use our test secure path
		// For integration testing, we disable security validation to focus on PATH resolution
		manager.pathResolver = NewPathResolver(testSecurePath, nil, false)

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

		mockFS := common.NewMockFileSystem()
		require.NoError(t, mockFS.AddDir(testHashDir, 0o755))

		manager, err := newManagerInternal(testHashDir,
			withFSInternal(mockFS),
			withFileValidatorDisabledInternal(),
			withCreationMode(CreationModeTesting),
			withSecurityLevel(SecurityLevelRelaxed))
		require.NoError(t, err)

		// For integration testing, we disable security validation to focus on PATH resolution
		manager.pathResolver = NewPathResolver(testSecurePath, nil, false)

		// Should find the first one in the PATH order (/sbin comes first)
		resolved, err := manager.ResolvePath("duplicate")
		require.NoError(t, err)
		assert.Equal(t, duplicateCmd1, resolved) // Should find /sbin/duplicate first
	})

	t.Run("integration with default securePathEnv structure", func(t *testing.T) {
		// Test that our Manager correctly uses the hardcoded securePathEnv
		// We can't easily test with the actual system paths, but we can verify
		// that the Manager uses its pathResolver correctly
		mockFS := common.NewMockFileSystem()
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
