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

// Ensure the internal option is referenced in non-test builds so linters
// don't report it as unused. Tests will actively use this option, but
// static analyzers run across packages/build tags and may flag it.
var _ = withSkipHashDirectoryValidationInternal
