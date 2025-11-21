// Package verification provides file hash verification and result collection
// for dry-run mode operations.
package verification

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
)

// Log level constants
const (
	logLevelInfo  = "info"
	logLevelWarn  = "warn"
	logLevelError = "error"
)

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
	// ReasonStandardPathSkipped indicates file was skipped due to standard system path
	ReasonStandardPathSkipped FailureReason = "standard_path_skipped"
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

// ResultCollector collects file verification results in dry-run mode
type ResultCollector struct {
	mu            sync.Mutex
	startTime     time.Time
	totalFiles    int
	verifiedFiles int
	skippedFiles  int
	failures      []FileVerificationFailure
	hashDirStatus HashDirectoryStatus
}

// NewResultCollector creates a new ResultCollector
func NewResultCollector(hashDirPath string) *ResultCollector {
	return &ResultCollector{
		startTime: time.Now(),
		hashDirStatus: HashDirectoryStatus{
			Path:      hashDirPath,
			Exists:    false,
			Validated: false,
		},
		failures: make([]FileVerificationFailure, 0),
	}
}

// RecordSuccess records a successful file verification
func (rc *ResultCollector) RecordSuccess(_ /* filePath */, _ /* context */ string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.totalFiles++
	rc.verifiedFiles++
}

// RecordFailure records a file verification failure
func (rc *ResultCollector) RecordFailure(filePath string, err error, context string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.totalFiles++

	reason := determineFailureReason(err)
	level := determineLogLevel(reason)

	failure := FileVerificationFailure{
		Path:    filePath,
		Reason:  reason,
		Level:   level,
		Message: err.Error(),
		Context: context,
	}

	rc.failures = append(rc.failures, failure)
}

// RecordSkip records a skipped file verification
func (rc *ResultCollector) RecordSkip(_ /* filePath */, _ /* context */ string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.totalFiles++
	rc.skippedFiles++
}

// SetHashDirStatus sets the hash directory status
func (rc *ResultCollector) SetHashDirStatus(exists bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.hashDirStatus.Exists = exists
	rc.hashDirStatus.Validated = true
}

// GetSummary returns the verification summary
func (rc *ResultCollector) GetSummary() FileVerificationSummary {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	duration := time.Since(rc.startTime)

	return FileVerificationSummary{
		TotalFiles:    rc.totalFiles,
		VerifiedFiles: rc.verifiedFiles,
		SkippedFiles:  rc.skippedFiles,
		FailedFiles:   len(rc.failures),
		Duration:      duration,
		HashDirStatus: rc.hashDirStatus,
		Failures:      rc.failures,
	}
}

// determineFailureReason determines the failure reason from an error
func determineFailureReason(err error) FailureReason {
	if err == nil {
		return ""
	}

	// Check for specific filevalidator errors
	if errors.Is(err, filevalidator.ErrHashFileNotFound) {
		return ReasonHashFileNotFound
	}
	if errors.Is(err, filevalidator.ErrMismatch) {
		return ReasonHashMismatch
	}
	if errors.Is(err, filevalidator.ErrHashDirNotExist) {
		return ReasonHashDirNotFound
	}

	// Check error message for permission denied (Go's standard error)
	errMsg := err.Error()
	if strings.Contains(errMsg, "permission denied") {
		return ReasonPermissionDenied
	}

	// Default to file read error for unknown errors
	return ReasonFileReadError
}

// determineLogLevel determines the log level based on failure reason
func determineLogLevel(reason FailureReason) string {
	switch reason {
	case ReasonHashDirNotFound, ReasonStandardPathSkipped:
		return logLevelInfo
	case ReasonHashFileNotFound:
		return logLevelWarn
	case ReasonHashMismatch, ReasonFileReadError, ReasonPermissionDenied:
		return logLevelError
	default:
		return logLevelWarn
	}
}

// getSecurityRisk determines the security risk level based on failure reason
func getSecurityRisk(reason FailureReason) string {
	switch reason {
	case ReasonHashMismatch:
		return "high"
	case ReasonHashFileNotFound, ReasonFileReadError, ReasonPermissionDenied:
		return "medium"
	case ReasonHashDirNotFound, ReasonStandardPathSkipped:
		return "low"
	default:
		return "medium"
	}
}
