//go:build test

package verification

import (
	"log/slog"
	"runtime"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// isCallerInTestFile checks if the caller is in a test file
// This prevents testing APIs from being called from production code
func isCallerInTestFile() bool {
	// Check up to 10 levels in the call stack
	for i := 2; i < 12; i++ {
		if _, file, _, ok := runtime.Caller(i); ok {
			// Check if the file is a test file
			if strings.HasSuffix(file, "_test.go") {
				return true
			}
			// Also allow calls from testing infrastructure files
			if strings.Contains(file, "/testing/") {
				return true
			}
		} else {
			break
		}
	}
	return false
}

// TestOption is a function type for configuring Manager instances for testing
type TestOption func(*managerInternalOptions)

// WithFS sets a custom file system for testing
func WithFS(fs common.FileSystem) TestOption {
	return func(opts *managerInternalOptions) {
		opts.fs = fs
	}
}

// WithFileValidatorDisabled disables file validation for testing
func WithFileValidatorDisabled() TestOption {
	return func(opts *managerInternalOptions) {
		opts.fileValidatorEnabled = false
	}
}

// WithPrivilegeManager sets a custom privilege manager for testing
func WithPrivilegeManager(privMgr runnertypes.PrivilegeManager) TestOption {
	return func(opts *managerInternalOptions) {
		opts.privilegeManager = privMgr
	}
}

// WithTestingSecurityLevel sets the security level to relaxed for testing
func WithTestingSecurityLevel() TestOption {
	return func(opts *managerInternalOptions) {
		opts.securityLevel = SecurityLevelRelaxed
	}
}

// NewManagerForTest creates a new verification manager for testing with a custom hash directory
// This API allows custom hash directories for testing purposes and uses relaxed security constraints
func NewManagerForTest(hashDir string, options ...TestOption) (*Manager, error) {
	// Verify that this API is being called from test code
	if !isCallerInTestFile() {
		if _, file, line, ok := runtime.Caller(1); ok {
			return nil, NewProductionAPIViolationError("NewManagerForTest", file, line)
		}
		return nil, NewProductionAPIViolationError("NewManagerForTest", "unknown", 0)
	}

	// Log testing manager creation for audit trail
	slog.Info("Testing verification manager created",
		"api", "NewManagerForTest",
		"hash_directory", hashDir,
		"security_level", "relaxed")

	// Convert TestOption to InternalOption
	internalOptions := []InternalOption{
		withCreationMode(CreationModeTesting),
		withSecurityLevel(SecurityLevelRelaxed),
	}

	// Apply test options
	for _, opt := range options {
		internalOpts := newInternalOptions()
		opt(internalOpts)

		// Convert to internal options
		if internalOpts.fs != nil {
			internalOptions = append(internalOptions, withFSInternal(internalOpts.fs))
		}
		if !internalOpts.fileValidatorEnabled {
			internalOptions = append(internalOptions, withFileValidatorDisabledInternal())
		}
		if internalOpts.privilegeManager != nil {
			// Will be used when privilege manager support is added
			// internalOptions = append(internalOptions, withPrivilegeManagerInternal(internalOpts.privilegeManager))
		}
		if internalOpts.securityLevel == SecurityLevelRelaxed {
			internalOptions = append(internalOptions, withSecurityLevel(SecurityLevelRelaxed))
		}
	}

	// Create manager with testing constraints (allows custom hash directory)
	return newManagerInternal(hashDir, internalOptions...)
}
