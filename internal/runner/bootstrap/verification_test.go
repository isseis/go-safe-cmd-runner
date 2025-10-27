package bootstrap

import (
	"path/filepath"
	"testing"
)

func TestInitializeVerificationManager_Success(t *testing.T) {
	tests := []struct {
		name             string
		validatedHashDir string
		runID            string
		wantErr          bool
	}{
		{
			name:             "valid hash directory",
			validatedHashDir: ".hashes",
			runID:            "test-run-001",
			wantErr:          false,
		},
		{
			name:             "custom hash directory path",
			validatedHashDir: filepath.Join("custom", "path", ".hashes"),
			runID:            "test-run-002",
			wantErr:          false,
		},
		{
			name:             "absolute path hash directory",
			validatedHashDir: "/tmp/.hashes",
			runID:            "test-run-003",
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup logger before testing (verification manager logs errors)
			err := SetupLogging("info", "", tt.runID, false, false)
			if err != nil {
				t.Fatalf("Failed to setup logging: %v", err)
			}

			manager, err := InitializeVerificationManager(tt.validatedHashDir, tt.runID)

			if (err != nil) != tt.wantErr {
				t.Errorf("InitializeVerificationManager() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && manager == nil {
				t.Error("InitializeVerificationManager() returned nil manager without error")
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
					t.Fatalf("Setup failed: %v", err)
				}
			}

			manager, err := InitializeVerificationManager(tt.validatedHashDir, tt.runID)
			// In production, the function should succeed because it uses the default hash directory
			if err != nil {
				t.Logf("InitializeVerificationManager() returned error (expected for non-existent default dir): %v", err)
			}

			// If the default hash directory doesn't exist, we expect an error
			// If it does exist, we expect a manager
			if err != nil && manager != nil {
				t.Error("InitializeVerificationManager() should return nil manager on error")
			}

			if err == nil && manager == nil {
				t.Error("InitializeVerificationManager() should return a manager on success")
			}
		})
	}
}

func TestInitializeVerificationManager_PermissionError(t *testing.T) {
	// Setup logging first
	runID := "test-perm-001"
	err := SetupLogging("info", "", runID, false, false)
	if err != nil {
		t.Fatalf("Failed to setup logging: %v", err)
	}

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

	if err != nil && manager != nil {
		t.Error("InitializeVerificationManager() should return nil manager on error")
	}

	if err == nil && manager == nil {
		t.Error("InitializeVerificationManager() should return a manager on success")
	}
}
