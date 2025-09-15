package verification

import (
	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// CreationMode represents how the Manager was created
type CreationMode int

const (
	// CreationModeProduction indicates the Manager was created using the production API
	CreationModeProduction CreationMode = iota
	// CreationModeTesting indicates the Manager was created using the testing API
	CreationModeTesting
)

// String returns a string representation of CreationMode
func (c CreationMode) String() string {
	switch c {
	case CreationModeProduction:
		return "production"
	case CreationModeTesting:
		return "testing"
	default:
		return "unknown"
	}
}

// SecurityLevel represents the security enforcement level
type SecurityLevel int

const (
	// SecurityLevelStrict enforces all security constraints
	SecurityLevelStrict SecurityLevel = iota
	// SecurityLevelRelaxed allows some flexibility for testing
	SecurityLevelRelaxed
)

// String returns a string representation of SecurityLevel
func (s SecurityLevel) String() string {
	switch s {
	case SecurityLevelStrict:
		return "strict"
	case SecurityLevelRelaxed:
		return "relaxed"
	default:
		return "unknown"
	}
}

// managerInternalOptions holds all configuration options for creating a Manager internally
type managerInternalOptions struct {
	fs                          common.FileSystem
	fileValidatorEnabled        bool
	creationMode                CreationMode
	securityLevel               SecurityLevel
	skipHashDirectoryValidation bool
	isDryRun                    bool
	customPathResolver          *PathResolver
}

func newInternalOptions() *managerInternalOptions {
	return &managerInternalOptions{
		fileValidatorEnabled: true,
		fs:                   common.NewDefaultFileSystem(),
		creationMode:         CreationModeProduction,
		securityLevel:        SecurityLevelStrict,
	}
}

// InternalOption is a function type for configuring Manager instances internally
type InternalOption func(*managerInternalOptions)

// withCreationMode sets the creation mode
func withCreationMode(mode CreationMode) InternalOption {
	return func(opts *managerInternalOptions) {
		opts.creationMode = mode
	}
}

// withSecurityLevel sets the security level
func withSecurityLevel(level SecurityLevel) InternalOption {
	return func(opts *managerInternalOptions) {
		opts.securityLevel = level
	}
}

// withFSInternal is an internal option for setting the file system
func withFSInternal(fs common.FileSystem) InternalOption {
	return func(opts *managerInternalOptions) {
		opts.fs = fs
	}
}

// withFileValidatorDisabledInternal is an internal option for disabling the file validator
func withFileValidatorDisabledInternal() InternalOption {
	return func(opts *managerInternalOptions) {
		opts.fileValidatorEnabled = false
	}
}

// withSkipHashDirectoryValidationInternal is an internal option for skipping hash directory validation
func withSkipHashDirectoryValidationInternal() InternalOption {
	return func(opts *managerInternalOptions) {
		opts.skipHashDirectoryValidation = true
	}
}

// withDryRunModeInternal is an internal option for marking the manager as dry-run mode
func withDryRunModeInternal() InternalOption {
	return func(opts *managerInternalOptions) {
		opts.isDryRun = true
	}
}

// withCustomPathResolverInternal is an internal option for setting a custom path resolver
func withCustomPathResolverInternal(pathResolver *PathResolver) InternalOption {
	return func(opts *managerInternalOptions) {
		opts.customPathResolver = pathResolver
	}
}

// Ensure the internal options are referenced in non-test builds so linters
// don't report them as unused. Tests will actively use these options, but
// static analyzers run across packages/build tags and may flag them.
var (
	_ = withSkipHashDirectoryValidationInternal
	_ = withCustomPathResolverInternal
)
