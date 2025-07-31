package filevalidator_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	privtesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
	"github.com/stretchr/testify/assert"
)

func TestNewValidatorWithPrivileges(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_privileged_validator")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	algorithm := &filevalidator.SHA256{}
	mockPrivMgr := privtesting.NewMockPrivilegeManager(true)
	logger := slog.Default()

	validator, err := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
	assert.NoError(t, err)
	assert.NotNil(t, validator)
}

func TestValidatorWithPrivileges_VerifyWithPrivileges(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_privileged_verify")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test file and record its hash first
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content for privileged verification"
	err = os.WriteFile(testFile, []byte(testContent), 0o644)
	assert.NoError(t, err)

	algorithm := &filevalidator.SHA256{}
	mockPrivMgr := privtesting.NewMockPrivilegeManager(true)
	logger := slog.Default()

	validator, err := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
	assert.NoError(t, err)

	// Record the hash first
	_, err = validator.Record(testFile)
	assert.NoError(t, err)

	tests := []struct {
		name            string
		needsPrivileges bool
		expectElevation bool
	}{
		{
			name:            "verify with privileges",
			needsPrivileges: true,
			expectElevation: true,
		},
		{
			name:            "verify without privileges",
			needsPrivileges: false,
			expectElevation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset elevation calls
			mockPrivMgr.ElevationCalls = nil

			ctx := context.Background()
			err := validator.VerifyWithPrivileges(ctx, testFile, tt.needsPrivileges)

			assert.NoError(t, err)

			if tt.expectElevation {
				assert.Contains(t, mockPrivMgr.ElevationCalls, "file_hash_calculation")
			} else {
				assert.Empty(t, mockPrivMgr.ElevationCalls)
			}
		})
	}
}

func TestValidatorWithPrivileges_ValidateFileHashWithPrivileges(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_privileged_validate")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content for privileged validation"
	err = os.WriteFile(testFile, []byte(testContent), 0o644)
	assert.NoError(t, err)

	// Calculate expected hash
	expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(testContent)))

	algorithm := &filevalidator.SHA256{}
	mockPrivMgr := privtesting.NewMockPrivilegeManager(true)
	logger := slog.Default()

	validator, err := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
	assert.NoError(t, err)

	tests := []struct {
		name            string
		needsPrivileges bool
		expectElevation bool
		expectedHash    string
		expectError     bool
	}{
		{
			name:            "validate with privileges - correct hash",
			needsPrivileges: true,
			expectElevation: true,
			expectedHash:    expectedHash,
			expectError:     false,
		},
		{
			name:            "validate without privileges - correct hash",
			needsPrivileges: false,
			expectElevation: false,
			expectedHash:    expectedHash,
			expectError:     false,
		},
		{
			name:            "validate with privileges - incorrect hash",
			needsPrivileges: true,
			expectElevation: true,
			expectedHash:    "invalidhash",
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset elevation calls
			mockPrivMgr.ElevationCalls = nil

			ctx := context.Background()
			err := validator.ValidateFileHashWithPrivileges(ctx, testFile, tt.expectedHash, tt.needsPrivileges)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectElevation {
				assert.Contains(t, mockPrivMgr.ElevationCalls, "file_hash_calculation")
			} else {
				assert.Empty(t, mockPrivMgr.ElevationCalls)
			}
		})
	}
}

func TestValidatorWithPrivileges_PrivilegeElevationFailure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_privilege_failure")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"
	err = os.WriteFile(testFile, []byte(testContent), 0o644)
	assert.NoError(t, err)

	algorithm := &filevalidator.SHA256{}
	mockPrivMgr := privtesting.NewFailingMockPrivilegeManager(true)
	logger := slog.Default()

	validator, err := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
	assert.NoError(t, err)

	ctx := context.Background()

	// Test VerifyWithPrivileges failure
	err = validator.VerifyWithPrivileges(ctx, testFile, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "privileged file hash verification failed")

	// Test ValidateFileHashWithPrivileges failure
	err = validator.ValidateFileHashWithPrivileges(ctx, testFile, "somehash", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "privileged file hash validation failed")
}

func TestValidatorWithPrivileges_PathValidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_path_validation")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	algorithm := &filevalidator.SHA256{}
	mockPrivMgr := privtesting.NewMockPrivilegeManager(true)
	logger := slog.Default()

	validator, err := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
	assert.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name         string
		filePath     string
		expectError  bool
		errorMessage string
	}{
		{
			name:         "empty file path should fail",
			filePath:     "",
			expectError:  true,
			errorMessage: "file path validation failed",
		},
		{
			name:         "non-existent file should fail",
			filePath:     "/non/existent/file.txt",
			expectError:  true,
			errorMessage: "file path validation failed",
		},
		{
			name:         "directory instead of file should fail",
			filePath:     tempDir,
			expectError:  true,
			errorMessage: "file path validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test ValidateFileHashWithPrivileges with invalid paths
			err := validator.ValidateFileHashWithPrivileges(ctx, tt.filePath, "somehash", false)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatorWithPrivileges_PrivilegesRequiredButUnavailable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_privileges_required")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"
	err = os.WriteFile(testFile, []byte(testContent), 0o644)
	assert.NoError(t, err)

	tests := []struct {
		name           string
		setupValidator func() *filevalidator.ValidatorWithPrivileges
		expectedError  error
	}{
		{
			name: "privileges required but no privilege manager",
			setupValidator: func() *filevalidator.ValidatorWithPrivileges {
				// Use NewValidatorWithPrivileges with nil privilege manager
				algorithm := &filevalidator.SHA256{}
				logger := slog.Default()
				validator, _ := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, nil, logger)
				return validator
			},
			expectedError: filevalidator.ErrPrivilegesRequiredButNoManager,
		},
		{
			name: "privileges required but not supported",
			setupValidator: func() *filevalidator.ValidatorWithPrivileges {
				// Mock privilege manager that doesn't support privileges
				mockPrivMgr := privtesting.NewMockPrivilegeManager(false) // Not supported
				algorithm := &filevalidator.SHA256{}
				logger := slog.Default()
				validator, _ := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
				return validator
			},
			expectedError: filevalidator.ErrPrivilegesRequiredButNotSupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := tt.setupValidator()
			ctx := context.Background()

			// Test VerifyWithPrivileges with needsPrivileges=true
			err = validator.VerifyWithPrivileges(ctx, testFile, true)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, tt.expectedError), "Expected error %v, got %v", tt.expectedError, err)

			// Test ValidateFileHashWithPrivileges with needsPrivileges=true
			err = validator.ValidateFileHashWithPrivileges(ctx, testFile, "somehash", true)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, tt.expectedError), "Expected error %v, got %v", tt.expectedError, err)
		})
	}
}
