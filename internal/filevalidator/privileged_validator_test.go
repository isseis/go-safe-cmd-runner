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
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/stretchr/testify/assert"
)

// MockPrivilegeManager for testing privileged file operations
type MockPrivilegeManager struct {
	supported         bool
	elevationCalls    []string
	shouldFail        bool
	withPrivilegeFunc func() error
}

// Test error definitions
var (
	ErrMockPrivilegeElevationFailed = errors.New("mock privilege elevation failure")
)

func (m *MockPrivilegeManager) WithPrivileges(_ context.Context, elevationCtx privilege.ElevationContext, fn func() error) error {
	m.elevationCalls = append(m.elevationCalls, string(elevationCtx.Operation))
	if m.shouldFail {
		return ErrMockPrivilegeElevationFailed
	}
	if m.withPrivilegeFunc != nil {
		return m.withPrivilegeFunc()
	}
	return fn()
}

func (m *MockPrivilegeManager) IsPrivilegedExecutionSupported() bool {
	return m.supported
}

func (m *MockPrivilegeManager) GetCurrentUID() int {
	return 1000
}

func (m *MockPrivilegeManager) GetOriginalUID() int {
	return 1000
}

func (m *MockPrivilegeManager) HealthCheck(_ context.Context) error {
	if !m.supported {
		return privilege.ErrPrivilegedExecutionNotAvailable
	}
	return nil
}

func (m *MockPrivilegeManager) GetHealthStatus(_ context.Context) privilege.HealthStatus {
	return privilege.HealthStatus{
		IsSupported:      m.supported,
		SetuidConfigured: m.supported,
		OriginalUID:      1000,
		CurrentUID:       1000,
		EffectiveUID:     1000,
		CanElevate:       m.supported,
	}
}

func (m *MockPrivilegeManager) GetMetrics() privilege.Metrics {
	return privilege.Metrics{}
}

func TestNewValidatorWithPrivileges(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test_privileged_validator")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	algorithm := &filevalidator.SHA256{}
	mockPrivMgr := &MockPrivilegeManager{supported: true}
	logger := slog.Default()

	validator, err := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
	assert.NoError(t, err)
	assert.NotNil(t, validator)
}

func TestValidatorWithPrivileges_RecordWithPrivileges(t *testing.T) {
	tests := []struct {
		name            string
		needsPrivileges bool
		expectElevation bool
	}{
		{
			name:            "record with privileges",
			needsPrivileges: true,
			expectElevation: true,
		},
		{
			name:            "record without privileges",
			needsPrivileges: false,
			expectElevation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create separate temp directory for each test to avoid conflicts
			tempDir, err := os.MkdirTemp("", "test_privileged_record")
			assert.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// Create test file
			testFile := filepath.Join(tempDir, "test.txt")
			testContent := "test content for privileged recording"
			err = os.WriteFile(testFile, []byte(testContent), 0o644)
			assert.NoError(t, err)

			algorithm := &filevalidator.SHA256{}
			mockPrivMgr := &MockPrivilegeManager{supported: true}
			logger := slog.Default()

			validator, err := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
			assert.NoError(t, err)

			// Reset elevation calls
			mockPrivMgr.elevationCalls = nil

			ctx := context.Background()
			hash, err := validator.RecordWithPrivileges(ctx, testFile, tt.needsPrivileges, false)

			assert.NoError(t, err)
			assert.NotEmpty(t, hash, "Hash should not be empty for file %s", testFile)

			if tt.expectElevation {
				assert.Contains(t, mockPrivMgr.elevationCalls, "file_hash_calculation")
			} else {
				assert.Empty(t, mockPrivMgr.elevationCalls)
			}
		})
	}
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
	mockPrivMgr := &MockPrivilegeManager{supported: true}
	logger := slog.Default()

	validator, err := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
	assert.NoError(t, err)

	// Record the hash first
	_, err = validator.RecordWithPrivileges(context.Background(), testFile, false, false)
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
			mockPrivMgr.elevationCalls = nil

			ctx := context.Background()
			err := validator.VerifyWithPrivileges(ctx, testFile, tt.needsPrivileges)

			assert.NoError(t, err)

			if tt.expectElevation {
				assert.Contains(t, mockPrivMgr.elevationCalls, "file_hash_calculation")
			} else {
				assert.Empty(t, mockPrivMgr.elevationCalls)
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
	mockPrivMgr := &MockPrivilegeManager{supported: true}
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
			mockPrivMgr.elevationCalls = nil

			ctx := context.Background()
			err := validator.ValidateFileHashWithPrivileges(ctx, testFile, tt.expectedHash, tt.needsPrivileges)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectElevation {
				assert.Contains(t, mockPrivMgr.elevationCalls, "file_hash_calculation")
			} else {
				assert.Empty(t, mockPrivMgr.elevationCalls)
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
	mockPrivMgr := &MockPrivilegeManager{
		supported:  true,
		shouldFail: true, // Force privilege elevation to fail
	}
	logger := slog.Default()

	validator, err := filevalidator.NewValidatorWithPrivileges(algorithm, tempDir, mockPrivMgr, logger)
	assert.NoError(t, err)

	ctx := context.Background()

	// Test RecordWithPrivileges failure
	_, err = validator.RecordWithPrivileges(ctx, testFile, true, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "privileged file hash recording failed")

	// Test VerifyWithPrivileges failure
	err = validator.VerifyWithPrivileges(ctx, testFile, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "privileged file hash verification failed")

	// Test ValidateFileHashWithPrivileges failure
	err = validator.ValidateFileHashWithPrivileges(ctx, testFile, "somehash", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "privileged file hash validation failed")
}
