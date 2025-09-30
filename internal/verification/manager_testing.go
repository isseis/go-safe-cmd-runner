package verification

import (
	"log/slog"
	"runtime"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// isCallerInTestFile checks if the caller is in a test file
// This prevents testing APIs from being called from production code
func isCallerInTestFile() bool {
	// Iterate through the entire call stack until no more frames are available
	for i := 2; ; i++ {
		_, file, _, ok := runtime.Caller(i)
		if !ok {
			// No more stack frames available
			break
		}

		// Check if the file is a test file
		if strings.HasSuffix(file, "_test.go") {
			return true
		}
		// Also allow calls from testing infrastructure files
		if strings.Contains(file, "/testing/") {
			return true
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

// WithFileValidatorEnabled enables file validation for testing
func WithFileValidatorEnabled() TestOption {
	return func(opts *managerInternalOptions) {
		opts.fileValidatorEnabled = true
	}
}

// WithTestingSecurityLevel sets the security level to relaxed for testing
func WithTestingSecurityLevel() TestOption {
	return func(opts *managerInternalOptions) {
		opts.securityLevel = SecurityLevelRelaxed
	}
}

// WithSkipHashDirectoryValidation skips hash directory validation for testing
func WithSkipHashDirectoryValidation() TestOption {
	return func(opts *managerInternalOptions) {
		opts.skipHashDirectoryValidation = true
	}
}

// WithPathResolver sets a custom path resolver for testing
func WithPathResolver(pathResolver *PathResolver) TestOption {
	return func(opts *managerInternalOptions) {
		opts.customPathResolver = pathResolver
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

	// Start with default testing options
	internalOpts := newInternalOptions()
	internalOpts.creationMode = CreationModeTesting
	internalOpts.securityLevel = SecurityLevelRelaxed
	internalOpts.skipHashDirectoryValidation = true
	// Keep fileValidatorEnabled = true by default for proper testing

	// Apply user-provided options
	for _, opt := range options {
		opt(internalOpts)
	}

	// Convert to InternalOption array
	internalOptions := []InternalOption{
		withCreationMode(internalOpts.creationMode),
		withSecurityLevel(internalOpts.securityLevel),
	}

	if internalOpts.skipHashDirectoryValidation {
		internalOptions = append(internalOptions, withSkipHashDirectoryValidationInternal())
	}

	if !internalOpts.fileValidatorEnabled {
		internalOptions = append(internalOptions, withFileValidatorDisabledInternal())
	}

	if internalOpts.fs != nil {
		internalOptions = append(internalOptions, withFSInternal(internalOpts.fs))
	}

	if internalOpts.customPathResolver != nil {
		internalOptions = append(internalOptions, withCustomPathResolverInternal(internalOpts.customPathResolver))
	}

	// Create manager with testing constraints (allows custom hash directory)
	return newManagerInternal(hashDir, internalOptions...)
}
