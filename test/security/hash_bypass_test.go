//go:build test
// +build test

package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/stretchr/testify/require"
)

// TestHashValidation_BasicBypassAttempts tests basic file hash validation bypass protection
func TestHashValidation_BasicBypassAttempts(t *testing.T) {
	tempDir := t.TempDir()
	hashDir := filepath.Join(tempDir, "hashes")

	// Create hash directory
	require.NoError(t, os.MkdirAll(hashDir, 0o755))

	algo := &filevalidator.SHA256{}
	validator, err := filevalidator.New(algo, hashDir)
	require.NoError(t, err)

	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) (string, func())
		shouldPass  bool
		description string
	}{
		{
			name: "Valid file with matching hash",
			setupFunc: func(t *testing.T) (string, func()) {
				testFile := filepath.Join(tempDir, "valid_file.txt")
				testContent := "This is valid content\n"

				// Create test file
				require.NoError(t, os.WriteFile(testFile, []byte(testContent), 0o644),
					"Failed to create test file")

				// Record hash
				_, err := validator.Record(testFile, false)
				require.NoError(t, err, "Failed to record hash")

				cleanup := func() {
					os.Remove(testFile)
				}
				return testFile, cleanup
			},
			shouldPass:  true,
			description: "File with valid hash should pass verification",
		},
		{
			name: "Modified file after hash recording",
			setupFunc: func(t *testing.T) (string, func()) {
				testFile := filepath.Join(tempDir, "modified_file.txt")
				err := os.WriteFile(testFile, []byte("original content"), 0o644)
				require.NoError(t, err)

				_, err = validator.Record(testFile, false)
				require.NoError(t, err)

				// Modify file after recording hash
				err = os.WriteFile(testFile, []byte("modified content"), 0o644)
				require.NoError(t, err)

				return testFile, func() { os.Remove(testFile) }
			},
			shouldPass:  false,
			description: "Modified file should fail verification (hash mismatch)",
		},
		{
			name: "File without hash record",
			setupFunc: func(t *testing.T) (string, func()) {
				testFile := filepath.Join(tempDir, "no_hash.txt")
				err := os.WriteFile(testFile, []byte("content without hash"), 0o644)
				require.NoError(t, err)

				// Do NOT record hash
				return testFile, func() { os.Remove(testFile) }
			},
			shouldPass:  false,
			description: "File without hash should fail verification",
		},
		{
			name: "Non-existent file",
			setupFunc: func(_ *testing.T) (string, func()) {
				nonExistentFile := filepath.Join(tempDir, "non_existent.txt")
				return nonExistentFile, func() {}
			},
			shouldPass:  false,
			description: "Non-existent file should fail verification",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile, cleanup := tt.setupFunc(t)
			defer cleanup()

			err := validator.Verify(testFile)
			if tt.shouldPass {
				require.NoError(t, err, tt.description)
				t.Logf("Verification passed as expected: %s", tt.description)
			} else {
				require.Error(t, err, tt.description)
				t.Logf("Verification failed as expected: %s (error: %v)", tt.description, err)
			}
		})
	}
}

// TestHashValidation_ManifestTampering tests protection against hash manifest tampering
func TestHashValidation_ManifestTampering(t *testing.T) {
	tempDir := t.TempDir()
	hashDir := filepath.Join(tempDir, "hashes")

	// Create hash directory
	require.NoError(t, os.MkdirAll(hashDir, 0o755))

	algo := &filevalidator.SHA256{}
	validator, err := filevalidator.New(algo, hashDir)
	require.NoError(t, err)

	tests := []struct {
		name       string
		setupFunc  func(t *testing.T) string
		tamperFunc func(t *testing.T, testFile string)
	}{
		{
			name: "Tampered hash value in manifest",
			setupFunc: func(t *testing.T) string {
				testFile := filepath.Join(tempDir, "tamper_hash.txt")
				err := os.WriteFile(testFile, []byte("original content"), 0o644)
				require.NoError(t, err)

				_, err = validator.Record(testFile, false)
				require.NoError(t, err)

				return testFile
			},
			tamperFunc: func(t *testing.T, testFile string) {
				// Get the hash file path
				absPath, err := filepath.Abs(testFile)
				require.NoError(t, err)
				resolvedPathStr, err := filepath.EvalSymlinks(absPath)
				require.NoError(t, err)

				resolvedPath, err := common.NewResolvedPath(resolvedPathStr)
				require.NoError(t, err)

				// Use internal method to get hash file path
				hashFilePath, err := validator.GetHashFilePath(resolvedPath)
				require.NoError(t, err)

				// Tamper with the hash value by modifying the JSON content
				// Replace the hash value with a fake one
				tamperedContent := []byte(`{
  "version": "1.0",
  "file": {
    "path": "` + resolvedPathStr + `",
    "hash": {
      "algorithm": "SHA-256",
      "value": "0000000000000000000000000000000000000000000000000000000000000000"
    }
  }
}
`)
				err = os.WriteFile(hashFilePath, tamperedContent, 0o644)
				require.NoError(t, err)
				t.Logf("Tampered hash manifest file: %s", hashFilePath)
			},
		},
		{
			name: "Deleted hash file",
			setupFunc: func(t *testing.T) string {
				testFile := filepath.Join(tempDir, "deleted_hash.txt")
				err := os.WriteFile(testFile, []byte("content"), 0o644)
				require.NoError(t, err)

				_, err = validator.Record(testFile, false)
				require.NoError(t, err)

				return testFile
			},
			tamperFunc: func(t *testing.T, testFile string) {
				// Get the hash file path
				absPath, err := filepath.Abs(testFile)
				require.NoError(t, err)
				resolvedPathStr, err := filepath.EvalSymlinks(absPath)
				require.NoError(t, err)

				resolvedPath, err := common.NewResolvedPath(resolvedPathStr)
				require.NoError(t, err)

				hashFilePath, err := validator.GetHashFilePath(resolvedPath)
				require.NoError(t, err)

				// Delete the hash file
				err = os.Remove(hashFilePath)
				require.NoError(t, err)
				t.Logf("Deleted hash file: %s", hashFilePath)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := tt.setupFunc(t)
			defer os.Remove(testFile)

			// First verification should pass (before tampering)
			err := validator.Verify(testFile)
			require.NoError(t, err)
			t.Logf("Initial verification passed")

			// Apply tampering
			tt.tamperFunc(t, testFile)

			// Verification should fail after tampering
			err = validator.Verify(testFile)
			require.Error(t, err, "Verification should fail after tampering")
			t.Logf("Verification correctly failed after tampering: %v", err)
		})
	}
}

// TestHashValidation_SymbolicLinkAttack tests protection against symlink attacks
func TestHashValidation_SymbolicLinkAttack(t *testing.T) {
	tempDir := t.TempDir()
	hashDir := filepath.Join(tempDir, "hashes")

	// Create hash directory
	require.NoError(t, os.MkdirAll(hashDir, 0o755))

	algo := &filevalidator.SHA256{}
	validator, err := filevalidator.New(algo, hashDir)
	require.NoError(t, err)

	// Create a legitimate file
	legitFile := filepath.Join(tempDir, "legit_file.txt")
	err = os.WriteFile(legitFile, []byte("legitimate content"), 0o644)
	require.NoError(t, err)
	defer os.Remove(legitFile)

	// Record hash for legitimate file
	_, err = validator.Record(legitFile, false)
	require.NoError(t, err)

	// Verify legitimate file
	err = validator.Verify(legitFile)
	require.NoError(t, err)
	t.Logf("Legitimate file verification passed")

	// Create a sensitive file
	sensitiveFile := filepath.Join(tempDir, "sensitive.txt")
	err = os.WriteFile(sensitiveFile, []byte("sensitive data"), 0o600)
	require.NoError(t, err)
	defer os.Remove(sensitiveFile)

	// Try to create a symlink pointing to the sensitive file
	symlinkPath := filepath.Join(tempDir, "symlink.txt")
	err = os.Symlink(sensitiveFile, symlinkPath)
	if err != nil {
		t.Skipf("Cannot create symlinks: %v", err)
	}
	defer os.Remove(symlinkPath)

	// Verify symlink - should fail because sensitive.txt has no recorded hash
	// The validator should either:
	// 1. Reject symlinks explicitly, or
	// 2. Follow the symlink and fail because no hash exists for the target
	err = validator.Verify(symlinkPath)
	require.Error(t, err, "Symlink verification should fail: target file has no recorded hash")
	t.Logf("Symlink verification failed as expected: %v", err)
}

// TestHashValidation_RaceConditionProtection tests TOCTOU protection
func TestHashValidation_RaceConditionProtection(t *testing.T) {
	tempDir := t.TempDir()
	hashDir := filepath.Join(tempDir, "hashes")

	// Create hash directory
	require.NoError(t, os.MkdirAll(hashDir, 0o755))

	algo := &filevalidator.SHA256{}
	validator, err := filevalidator.New(algo, hashDir)
	require.NoError(t, err)

	testFile := filepath.Join(tempDir, "toctou_test.txt")
	originalContent := []byte("original content")
	err = os.WriteFile(testFile, originalContent, 0o644)
	require.NoError(t, err)
	defer os.Remove(testFile)

	// Record hash
	_, err = validator.Record(testFile, false)
	require.NoError(t, err)

	// Verify the file
	err = validator.Verify(testFile)
	require.NoError(t, err)
	t.Logf("Initial verification passed")

	// Simulate TOCTOU attack: modify file after verification but before use
	modifiedContent := []byte("modified after verification")
	err = os.WriteFile(testFile, modifiedContent, 0o644)
	require.NoError(t, err)

	// Verify again - should fail because content changed
	err = validator.Verify(testFile)
	require.Error(t, err, "TOCTOU protection: modified file should fail verification")
	t.Logf("TOCTOU protection: modification detected (%v)", err)
}
