// Package verification provides file hash verification and result collection
// for dry-run mode operations.
package verification

import (
	"errors"
	"log/slog"
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
func (rc *ResultCollector) RecordSuccess() {
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
func (rc *ResultCollector) RecordSkip() {
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

	// Deep copy failures slice to prevent data races
	failuresCopy := make([]FileVerificationFailure, len(rc.failures))
	copy(failuresCopy, rc.failures)

	return FileVerificationSummary{
		TotalFiles:    rc.totalFiles,
		VerifiedFiles: rc.verifiedFiles,
		SkippedFiles:  rc.skippedFiles,
		FailedFiles:   len(rc.failures),
		Duration:      duration,
		HashDirStatus: rc.hashDirStatus,
		Failures:      failuresCopy,
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

// logVerificationFailure logs a verification failure with appropriate log level based on failure reason
func logVerificationFailure(filePath, context string, err error, operation string) {
	reason := determineFailureReason(err)
	level := determineLogLevel(reason)

	attrs := []any{
		"file_path", filePath,
		"context", context,
		"reason", reason,
		"security_risk", getSecurityRisk(reason),
		"error", err,
	}

	switch level {
	case logLevelError:
		slog.Error(operation+" failed in dry-run mode", attrs...)
	case logLevelWarn:
		slog.Warn(operation+" issue in dry-run mode", attrs...)
	default:
		slog.Info(operation+" skipped in dry-run mode", attrs...)
	}
}
