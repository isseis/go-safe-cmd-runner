//go:build test

package verification

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResultCollector(t *testing.T) {
	hashDirPath := "/usr/local/etc/go-safe-cmd-runner/hashes"
	rc := NewResultCollector(hashDirPath)

	require.NotNil(t, rc, "NewResultCollector returned nil")

	summary := rc.GetSummary()

	assert.Equal(t, 0, summary.TotalFiles, "TotalFiles should be 0")
	assert.Equal(t, 0, summary.VerifiedFiles, "VerifiedFiles should be 0")
	assert.Equal(t, 0, summary.SkippedFiles, "SkippedFiles should be 0")
	assert.Equal(t, 0, summary.FailedFiles, "FailedFiles should be 0")
	assert.Equal(t, hashDirPath, summary.HashDirStatus.Path, "HashDirStatus.Path mismatch")
	assert.False(t, summary.HashDirStatus.Exists, "HashDirStatus.Exists should be false")
	assert.False(t, summary.HashDirStatus.Validated, "HashDirStatus.Validated should be false")
	assert.Equal(t, 0, len(summary.Failures), "Failures should be empty")
}

func TestResultCollector_RecordSuccess(t *testing.T) {
	rc := NewResultCollector("/test/path")

	rc.RecordSuccess()
	rc.RecordSuccess()

	summary := rc.GetSummary()

	assert.Equal(t, 2, summary.TotalFiles, "TotalFiles should be 2")
	assert.Equal(t, 2, summary.VerifiedFiles, "VerifiedFiles should be 2")
	assert.Equal(t, 0, summary.SkippedFiles, "SkippedFiles should be 0")
	assert.Equal(t, 0, summary.FailedFiles, "FailedFiles should be 0")
}

func TestResultCollector_RecordFailure(t *testing.T) {
	rc := NewResultCollector("/test/path")

	err1 := filevalidator.ErrHashFileNotFound
	rc.RecordFailure("/path/to/file1.toml", err1, "config")

	err2 := filevalidator.ErrMismatch
	rc.RecordFailure("/path/to/file2.toml", err2, "global")

	summary := rc.GetSummary()

	assert.Equal(t, 2, summary.TotalFiles, "TotalFiles should be 2")
	assert.Equal(t, 0, summary.VerifiedFiles, "VerifiedFiles should be 0")
	assert.Equal(t, 0, summary.SkippedFiles, "SkippedFiles should be 0")
	assert.Equal(t, 2, summary.FailedFiles, "FailedFiles should be 2")

	// Check failures
	require.Equal(t, 2, len(summary.Failures), "expected 2 failures")

	// First failure
	f1 := summary.Failures[0]
	assert.Equal(t, "/path/to/file1.toml", f1.Path)
	assert.Equal(t, ReasonHashFileNotFound, f1.Reason)
	assert.Equal(t, logLevelError, f1.Level) // ERROR because it would fail in production
	assert.Equal(t, "config", f1.Context)

	// Second failure
	f2 := summary.Failures[1]
	assert.Equal(t, "/path/to/file2.toml", f2.Path)
	assert.Equal(t, ReasonHashMismatch, f2.Reason)
	assert.Equal(t, logLevelError, f2.Level)
	assert.Equal(t, "global", f2.Context)
}

func TestResultCollector_RecordSkip(t *testing.T) {
	rc := NewResultCollector("/test/path")

	rc.RecordSkip()
	rc.RecordSkip()

	summary := rc.GetSummary()

	assert.Equal(t, 2, summary.TotalFiles, "TotalFiles should be 2")
	assert.Equal(t, 0, summary.VerifiedFiles, "VerifiedFiles should be 0")
	assert.Equal(t, 2, summary.SkippedFiles, "SkippedFiles should be 2")
	assert.Equal(t, 0, summary.FailedFiles, "FailedFiles should be 0")
}

func TestResultCollector_SetHashDirStatus(t *testing.T) {
	rc := NewResultCollector("/test/path")

	rc.SetHashDirStatus(true)

	summary := rc.GetSummary()

	assert.True(t, summary.HashDirStatus.Exists, "HashDirStatus.Exists should be true")
	assert.True(t, summary.HashDirStatus.Validated, "HashDirStatus.Validated should be true")
}

func TestResultCollector_GetSummary(t *testing.T) {
	rc := NewResultCollector("/test/path")

	// Record various outcomes
	rc.RecordSuccess()
	rc.RecordSuccess()
	rc.RecordFailure("/path/to/fail1.toml", filevalidator.ErrMismatch, "config")
	rc.RecordSkip()

	summary := rc.GetSummary()

	// Verify invariant: TotalFiles = VerifiedFiles + SkippedFiles + FailedFiles
	expectedTotal := summary.VerifiedFiles + summary.SkippedFiles + summary.FailedFiles
	assert.Equal(t, expectedTotal, summary.TotalFiles, "invariant violation: TotalFiles should equal sum of parts")

	// Verify invariant: FailedFiles = len(Failures)
	assert.Equal(t, len(summary.Failures), summary.FailedFiles, "FailedFiles should equal length of Failures")

	// Verify Duration
	assert.Greater(t, summary.Duration, time.Duration(0), "Duration should be > 0")
}

func TestResultCollector_Concurrency(t *testing.T) {
	rc := NewResultCollector("/test/path")

	const numGoroutines = 100
	const numOpsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < numOpsPerGoroutine; j++ {
				switch j % 3 {
				case 0:
					rc.RecordSuccess()
				case 1:
					rc.RecordFailure("/path/to/file", filevalidator.ErrHashFileNotFound, "test")
				case 2:
					rc.RecordSkip()
				}
			}
		}()
	}

	wg.Wait()

	summary := rc.GetSummary()

	expectedTotal := numGoroutines * numOpsPerGoroutine
	assert.Equal(t, expectedTotal, summary.TotalFiles, "TotalFiles mismatch after concurrent operations")

	// Verify invariant
	actualTotal := summary.VerifiedFiles + summary.SkippedFiles + summary.FailedFiles
	assert.Equal(t, actualTotal, summary.TotalFiles, "invariant violation after concurrent operations")
}

func TestDetermineFailureReason(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected FailureReason
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "hash file not found",
			err:      filevalidator.ErrHashFileNotFound,
			expected: ReasonHashFileNotFound,
		},
		{
			name:     "hash mismatch",
			err:      filevalidator.ErrMismatch,
			expected: ReasonHashMismatch,
		},
		{
			name:     "hash directory not exist",
			err:      filevalidator.ErrHashDirNotExist,
			expected: ReasonHashDirNotFound,
		},
		{
			name:     "permission denied",
			err:      os.ErrPermission,
			expected: ReasonPermissionDenied,
		},
		{
			name:     "unknown error",
			err:      errors.New("some unknown error"),
			expected: ReasonFileReadError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineFailureReason(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineLogLevel(t *testing.T) {
	tests := []struct {
		reason   FailureReason
		expected string
	}{
		{ReasonHashDirNotFound, logLevelInfo},
		{ReasonHashFileNotFound, logLevelError},
		{ReasonHashMismatch, logLevelError},
		{ReasonFileReadError, logLevelError},
		{ReasonPermissionDenied, logLevelError},
		{FailureReason("unknown"), logLevelWarn},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			result := determineLogLevel(tt.reason)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSecurityRisk(t *testing.T) {
	tests := []struct {
		reason   FailureReason
		expected string
	}{
		{ReasonHashMismatch, "high"},
		{ReasonHashFileNotFound, "medium"},
		{ReasonFileReadError, "medium"},
		{ReasonPermissionDenied, "medium"},
		{ReasonHashDirNotFound, "low"},
		{FailureReason("unknown"), "medium"},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			result := getSecurityRisk(tt.reason)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResultCollector_Duration(t *testing.T) {
	rc := NewResultCollector("/test/path")

	// Sleep to ensure duration is measurable
	time.Sleep(10 * time.Millisecond)

	summary := rc.GetSummary()

	assert.GreaterOrEqual(t, summary.Duration, 10*time.Millisecond, "Duration should be at least 10ms")
}

func TestResultCollector_MixedResults(t *testing.T) {
	rc := NewResultCollector("/test/path")

	// Simulate a real scenario with mixed results
	rc.RecordSuccess()
	rc.RecordSuccess()
	rc.RecordSuccess()
	rc.RecordFailure("/etc/global3.toml", filevalidator.ErrHashFileNotFound, "global")
	rc.RecordFailure("/etc/group.toml", filevalidator.ErrMismatch, "group:admin")
	rc.RecordSkip()
	rc.RecordSkip()
	rc.SetHashDirStatus(true)

	summary := rc.GetSummary()

	assert.Equal(t, 7, summary.TotalFiles)
	assert.Equal(t, 3, summary.VerifiedFiles)
	assert.Equal(t, 2, summary.SkippedFiles)
	assert.Equal(t, 2, summary.FailedFiles)

	// Check hash directory status
	assert.True(t, summary.HashDirStatus.Exists, "HashDirStatus.Exists should be true")
	assert.True(t, summary.HashDirStatus.Validated, "HashDirStatus.Validated should be true")

	// Check failures details
	require.Equal(t, 2, len(summary.Failures), "expected 2 failures")

	// Verify first failure (ERROR level - hash file not found would fail in production)
	assert.Equal(t, logLevelError, summary.Failures[0].Level)

	// Verify second failure (ERROR level - hash mismatch)
	assert.Equal(t, logLevelError, summary.Failures[1].Level)
}
