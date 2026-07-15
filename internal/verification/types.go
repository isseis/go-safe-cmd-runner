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

// SecurityLevel represents the security enforcement level
type SecurityLevel int

const (
	// SecurityLevelStrict enforces all security constraints
	SecurityLevelStrict SecurityLevel = iota
	// SecurityLevelRelaxed allows some flexibility for testing
	SecurityLevelRelaxed
)

// managerInternalOptions holds all configuration options for creating a Manager internally
type managerInternalOptions struct {
	fs                          common.FileSystem
	fileValidatorEnabled        bool
	creationMode                CreationMode
	securityLevel               SecurityLevel
	skipHashDirectoryValidation bool
	isDryRun                    bool
	customPathResolver          *PathResolver
	directoryValidator          DirectoryValidator
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

// GroupVerificationInput contains the minimum group data needed for verification.
type GroupVerificationInput struct {
	Name                string
	ExpandedVerifyFiles []string
	Commands            []CommandEntry
}

// CommandEntry represents a command input that may resolve to a file to verify.
type CommandEntry struct {
	ExpandedCmd string
}

// GlobalVerificationInput contains the minimum global data needed for verification.
type GlobalVerificationInput struct {
	ExpandedVerifyFiles []string
}

// DirectoryValidator validates hash directory permissions.
type DirectoryValidator interface {
	ValidateDirectoryPermissions(dirPath string) error
}

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

// withDirectoryValidatorInternal injects the directory validator used by production mode.
func withDirectoryValidatorInternal(validator DirectoryValidator) InternalOption {
	return func(opts *managerInternalOptions) {
		opts.directoryValidator = validator
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

// UnverifiedReason explains why a file's content was adopted without
// successful hash verification. It is distinct from FailureReason because
// the two are recorded in different states: FailureReason describes a
// verification that was attempted and reported, while UnverifiedReason
// explains why verification was skipped (no validator) or fell through
// (verification failed but dry-run continued with the unverified content).
type UnverifiedReason string

const (
	// UnverifiedReasonNoValidator indicates the file was adopted because no
	// file validator was configured for this manager instance (e.g. dry-run
	// on a machine where the hash directory is not writable).
	UnverifiedReasonNoValidator UnverifiedReason = "skipped_no_validator"

	// unverifiedReasonVerifyFailedPrefix prefixes the underlying FailureReason
	// to form the UnverifiedReason recorded when verification was attempted
	// and failed but the content was still adopted via the dry-run fallback.
	unverifiedReasonVerifyFailedPrefix = "verify_failed_"
)

// UnverifiedReasonFromFailure builds the UnverifiedReason recorded when
// verification was attempted and failed but the content was still adopted
// via the dry-run fallback. Centralized so the verify_failed_<reason> format
// has a single source of truth shared by production code and tests.
func UnverifiedReasonFromFailure(reason FailureReason) UnverifiedReason {
	return UnverifiedReason(unverifiedReasonVerifyFailedPrefix + string(reason))
}

// UnverifiedFileUsage records a single instance where a file's content was
// adopted by dry-run without successful hash verification. The reason is
// either UnverifiedReasonNoValidator or a verify_failed_<FailureReason>
// value (kept as a string to avoid coupling UnverifiedReason to the
// FailureReason enumeration).
type UnverifiedFileUsage struct {
	Path    string         `json:"path"`
	Reason  string         `json:"reason"`
	Context string         `json:"context"`
	Failure *FailureReason `json:"failure_reason,omitempty"`
}

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
	TotalFiles            int                       `json:"total_files"`
	VerifiedFiles         int                       `json:"verified_files"`
	FailedFiles           int                       `json:"failed_files"`
	Duration              time.Duration             `json:"duration"`
	HashDirStatus         HashDirectoryStatus       `json:"hash_dir_status"`
	Failures              []FileVerificationFailure `json:"failures,omitempty"`
	UsedUnverifiedContent bool                      `json:"used_unverified_content"`
	UnverifiedFiles       []UnverifiedFileUsage     `json:"unverified_files,omitempty"`
}
