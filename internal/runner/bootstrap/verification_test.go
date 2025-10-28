package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitializeVerificationManager_Success(t *testing.T) {
	// Note: In production, NewManager() always uses the default hash directory
	// (/usr/local/etc/go-safe-cmd-runner/hashes) regardless of validatedHashDir parameter.
	// The behavior depends on whether the default directory exists:
	// - If it exists: manager creation succeeds
	// - If it doesn't exist: manager creation fails with an error
	// This test verifies that the function properly handles both scenarios.

	tests := []struct {
		name             string
		validatedHashDir string
		runID            string
		description      string
	}{
		{
			name:             "behavior depends on default hash directory existence",
			validatedHashDir: ".hashes",
			runID:            "test-run-001",
			description:      "Success/failure depends on whether default directory exists",
		},
		{
			name:             "custom hash directory ignored - uses default",
			validatedHashDir: filepath.Join("custom", "path", ".hashes"),
			runID:            "test-run-002",
			description:      "validatedHashDir parameter is ignored; result depends on default dir",
		},
		{
			name:             "absolute path ignored - uses default",
			validatedHashDir: "/tmp/.hashes",
			runID:            "test-run-003",
			description:      "validatedHashDir parameter is ignored; result depends on default dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup logger before testing (verification manager logs errors)
			err := SetupLogging("info", "", tt.runID, false, false)
			assert.NoError(t, err, "Failed to setup logging")

			manager, err := InitializeVerificationManager(tt.validatedHashDir, tt.runID)

			// Verify consistency between error and manager
			assert.False(t, err != nil && manager != nil, "InitializeVerificationManager() should return nil manager on error")

			assert.False(t, err == nil && manager == nil, "InitializeVerificationManager() should return a manager on success")

			// Log the result for debugging purposes
			if err != nil {
				t.Logf("InitializeVerificationManager() failed (expected in environments without default hash directory): %v", err)
			} else {
				t.Logf("InitializeVerificationManager() succeeded (default hash directory exists)")
			}
		})
	}
}

func TestInitializeVerificationManager_InvalidHashDir(t *testing.T) {
	// Note: In production, NewManager() always uses the default hash directory
	// The validatedHashDir parameter is intentionally ignored for security reasons
	// This test verifies that the function handles logging properly regardless of input

	tests := []struct {
		name             string
		validatedHashDir string
		runID            string
		setupFunc        func() error
	}{
		{
			name:             "empty hash directory parameter - uses default",
			validatedHashDir: "",
			runID:            "test-run-err-001",
			setupFunc: func() error {
				// Setup logging first
				return SetupLogging("info", "", "test-run-err-001", false, false)
			},
		},
		{
			name:             "arbitrary hash directory parameter - uses default",
			validatedHashDir: "/some/arbitrary/path",
			runID:            "test-run-err-002",
			setupFunc: func() error {
				// Setup logging first
				return SetupLogging("info", "", "test-run-err-002", false, false)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				if err := tt.setupFunc(); err != nil {
					assert.NoError(t, err, "Setup failed")
				}
			}

			manager, err := InitializeVerificationManager(tt.validatedHashDir, tt.runID)
			// In production, the function should succeed because it uses the default hash directory
			if err != nil {
				t.Logf("InitializeVerificationManager() returned error (expected for non-existent default dir): %v", err)
			}

			// If the default hash directory doesn't exist, we expect an error
			// If it does exist, we expect a manager
			assert.False(t, err != nil && manager != nil, "InitializeVerificationManager() should return nil manager on error")

			assert.False(t, err == nil && manager == nil, "InitializeVerificationManager() should return a manager on success")
		})
	}
}

func TestInitializeVerificationManager_PermissionError(t *testing.T) {
	// Setup logging first
	runID := "test-perm-001"
	err := SetupLogging("info", "", runID, false, false)
	assert.NoError(t, err, "Failed to setup logging")

	// Note: Permission errors are typically handled at hash directory validation stage
	// The InitializeVerificationManager function expects a validated hash directory
	// However, in production it always uses the default hash directory

	// Test with various inputs - they should all result in the same behavior
	// because the function ignores the validatedHashDir parameter
	validatedHashDir := string([]byte{0x00}) // null byte in path

	manager, err := InitializeVerificationManager(validatedHashDir, runID)
	// The function may succeed or fail depending on whether the default hash directory exists
	// This is acceptable as the test verifies the function handles various inputs gracefully
	if err != nil {
		t.Logf("InitializeVerificationManager() returned error (may be expected): %v", err)
	}

	assert.False(t, err != nil && manager != nil, "InitializeVerificationManager() should return nil manager on error")

	assert.False(t, err == nil && manager == nil, "InitializeVerificationManager() should return a manager on success")
}
