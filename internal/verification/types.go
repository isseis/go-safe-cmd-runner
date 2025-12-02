package verification

import (
	"time"

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

// FailureReason represents the reason for verification failure
type FailureReason string

const (
	// ReasonHashDirNotFound indicates hash directory was not found
	ReasonHashDirNotFound FailureReason = "hash_directory_not_found"
	// ReasonHashFileNotFound indicates hash file for a specific file was not found
	ReasonHashFileNotFound FailureReason = "hash_file_not_found"
	// ReasonHashMismatch indicates hash value mismatch (potential tampering)
	ReasonHashMismatch FailureReason = "hash_mismatch"
	// ReasonFileReadError indicates file read operation failed
	ReasonFileReadError FailureReason = "file_read_error"
	// ReasonPermissionDenied indicates insufficient permissions to access file
	ReasonPermissionDenied FailureReason = "permission_denied"
)

// FileVerificationFailure represents a single file verification failure
type FileVerificationFailure struct {
	Path    string        `json:"path"`
	Reason  FailureReason `json:"reason"`
	Level   string        `json:"level"`
	Message string        `json:"message"`
	Context string        `json:"context"`
}

// HashDirectoryStatus represents the status of the hash directory
type HashDirectoryStatus struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	Validated bool   `json:"validated"`
}

// FileVerificationSummary represents the summary of file verification in dry-run mode
type FileVerificationSummary struct {
	TotalFiles    int                       `json:"total_files"`
	VerifiedFiles int                       `json:"verified_files"`
	SkippedFiles  int                       `json:"skipped_files"`
	FailedFiles   int                       `json:"failed_files"`
	Duration      time.Duration             `json:"duration"`
	HashDirStatus HashDirectoryStatus       `json:"hash_dir_status"`
	Failures      []FileVerificationFailure `json:"failures,omitempty"`
}
