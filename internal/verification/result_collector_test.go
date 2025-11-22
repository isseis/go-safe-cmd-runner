//go:build test

package verification

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
)

func TestNewResultCollector(t *testing.T) {
	hashDirPath := "/usr/local/etc/go-safe-cmd-runner/hashes"
	rc := NewResultCollector(hashDirPath)

	if rc == nil {
		t.Fatal("NewResultCollector returned nil")
	}

	summary := rc.GetSummary()

	if summary.TotalFiles != 0 {
		t.Errorf("expected TotalFiles = 0, got %d", summary.TotalFiles)
	}
	if summary.VerifiedFiles != 0 {
		t.Errorf("expected VerifiedFiles = 0, got %d", summary.VerifiedFiles)
	}
	if summary.SkippedFiles != 0 {
		t.Errorf("expected SkippedFiles = 0, got %d", summary.SkippedFiles)
	}
	if summary.FailedFiles != 0 {
		t.Errorf("expected FailedFiles = 0, got %d", summary.FailedFiles)
	}
	if summary.HashDirStatus.Path != hashDirPath {
		t.Errorf("expected HashDirStatus.Path = %s, got %s", hashDirPath, summary.HashDirStatus.Path)
	}
	if summary.HashDirStatus.Exists {
		t.Error("expected HashDirStatus.Exists = false")
	}
	if summary.HashDirStatus.Validated {
		t.Error("expected HashDirStatus.Validated = false")
	}
	if len(summary.Failures) != 0 {
		t.Errorf("expected empty Failures, got %d", len(summary.Failures))
	}
}

func TestResultCollector_RecordSuccess(t *testing.T) {
	rc := NewResultCollector("/test/path")

	rc.RecordSuccess()
	rc.RecordSuccess()

	summary := rc.GetSummary()

	if summary.TotalFiles != 2 {
		t.Errorf("expected TotalFiles = 2, got %d", summary.TotalFiles)
	}
	if summary.VerifiedFiles != 2 {
		t.Errorf("expected VerifiedFiles = 2, got %d", summary.VerifiedFiles)
	}
	if summary.SkippedFiles != 0 {
		t.Errorf("expected SkippedFiles = 0, got %d", summary.SkippedFiles)
	}
	if summary.FailedFiles != 0 {
		t.Errorf("expected FailedFiles = 0, got %d", summary.FailedFiles)
	}
}

func TestResultCollector_RecordFailure(t *testing.T) {
	rc := NewResultCollector("/test/path")

	err1 := filevalidator.ErrHashFileNotFound
	rc.RecordFailure("/path/to/file1.toml", err1, "config")

	err2 := filevalidator.ErrMismatch
	rc.RecordFailure("/path/to/file2.toml", err2, "global")

	summary := rc.GetSummary()

	if summary.TotalFiles != 2 {
		t.Errorf("expected TotalFiles = 2, got %d", summary.TotalFiles)
	}
	if summary.VerifiedFiles != 0 {
		t.Errorf("expected VerifiedFiles = 0, got %d", summary.VerifiedFiles)
	}
	if summary.SkippedFiles != 0 {
		t.Errorf("expected SkippedFiles = 0, got %d", summary.SkippedFiles)
	}
	if summary.FailedFiles != 2 {
		t.Errorf("expected FailedFiles = 2, got %d", summary.FailedFiles)
	}

	// Check failures
	if len(summary.Failures) != 2 {
		t.Fatalf("expected 2 failures, got %d", len(summary.Failures))
	}

	// First failure
	f1 := summary.Failures[0]
	if f1.Path != "/path/to/file1.toml" {
		t.Errorf("expected Path = /path/to/file1.toml, got %s", f1.Path)
	}
	if f1.Reason != ReasonHashFileNotFound {
		t.Errorf("expected Reason = %s, got %s", ReasonHashFileNotFound, f1.Reason)
	}
	if f1.Level != "warn" {
		t.Errorf("expected Level = warn, got %s", f1.Level)
	}
	if f1.Context != "config" {
		t.Errorf("expected Context = config, got %s", f1.Context)
	}

	// Second failure
	f2 := summary.Failures[1]
	if f2.Path != "/path/to/file2.toml" {
		t.Errorf("expected Path = /path/to/file2.toml, got %s", f2.Path)
	}
	if f2.Reason != ReasonHashMismatch {
		t.Errorf("expected Reason = %s, got %s", ReasonHashMismatch, f2.Reason)
	}
	if f2.Level != "error" {
		t.Errorf("expected Level = error, got %s", f2.Level)
	}
	if f2.Context != "global" {
		t.Errorf("expected Context = global, got %s", f2.Context)
	}
}

func TestResultCollector_RecordSkip(t *testing.T) {
	rc := NewResultCollector("/test/path")

	rc.RecordSkip()
	rc.RecordSkip()

	summary := rc.GetSummary()

	if summary.TotalFiles != 2 {
		t.Errorf("expected TotalFiles = 2, got %d", summary.TotalFiles)
	}
	if summary.VerifiedFiles != 0 {
		t.Errorf("expected VerifiedFiles = 0, got %d", summary.VerifiedFiles)
	}
	if summary.SkippedFiles != 2 {
		t.Errorf("expected SkippedFiles = 2, got %d", summary.SkippedFiles)
	}
	if summary.FailedFiles != 0 {
		t.Errorf("expected FailedFiles = 0, got %d", summary.FailedFiles)
	}
}

func TestResultCollector_SetHashDirStatus(t *testing.T) {
	rc := NewResultCollector("/test/path")

	rc.SetHashDirStatus(true)

	summary := rc.GetSummary()

	if !summary.HashDirStatus.Exists {
		t.Error("expected HashDirStatus.Exists = true")
	}
	if !summary.HashDirStatus.Validated {
		t.Error("expected HashDirStatus.Validated = true")
	}
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
	if summary.TotalFiles != expectedTotal {
		t.Errorf("invariant violation: TotalFiles (%d) != VerifiedFiles + SkippedFiles + FailedFiles (%d)",
			summary.TotalFiles, expectedTotal)
	}

	// Verify invariant: FailedFiles = len(Failures)
	if summary.FailedFiles != len(summary.Failures) {
		t.Errorf("invariant violation: FailedFiles (%d) != len(Failures) (%d)",
			summary.FailedFiles, len(summary.Failures))
	}

	// Verify Duration
	if summary.Duration <= 0 {
		t.Error("expected Duration > 0")
	}
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
	if summary.TotalFiles != expectedTotal {
		t.Errorf("expected TotalFiles = %d, got %d", expectedTotal, summary.TotalFiles)
	}

	// Verify invariant
	actualTotal := summary.VerifiedFiles + summary.SkippedFiles + summary.FailedFiles
	if summary.TotalFiles != actualTotal {
		t.Errorf("invariant violation after concurrent operations: TotalFiles (%d) != sum (%d)",
			summary.TotalFiles, actualTotal)
	}
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
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestDetermineLogLevel(t *testing.T) {
	tests := []struct {
		reason   FailureReason
		expected string
	}{
		{ReasonHashDirNotFound, "info"},
		{ReasonHashFileNotFound, "warn"},
		{ReasonHashMismatch, "error"},
		{ReasonFileReadError, "error"},
		{ReasonPermissionDenied, "error"},
		{FailureReason("unknown"), "warn"},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			result := determineLogLevel(tt.reason)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
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
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestResultCollector_Duration(t *testing.T) {
	rc := NewResultCollector("/test/path")

	// Sleep to ensure duration is measurable
	time.Sleep(10 * time.Millisecond)

	summary := rc.GetSummary()

	if summary.Duration < 10*time.Millisecond {
		t.Errorf("expected Duration >= 10ms, got %v", summary.Duration)
	}
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

	if summary.TotalFiles != 7 {
		t.Errorf("expected TotalFiles = 7, got %d", summary.TotalFiles)
	}
	if summary.VerifiedFiles != 3 {
		t.Errorf("expected VerifiedFiles = 3, got %d", summary.VerifiedFiles)
	}
	if summary.SkippedFiles != 2 {
		t.Errorf("expected SkippedFiles = 2, got %d", summary.SkippedFiles)
	}
	if summary.FailedFiles != 2 {
		t.Errorf("expected FailedFiles = 2, got %d", summary.FailedFiles)
	}

	// Check hash directory status
	if !summary.HashDirStatus.Exists {
		t.Error("expected HashDirStatus.Exists = true")
	}
	if !summary.HashDirStatus.Validated {
		t.Error("expected HashDirStatus.Validated = true")
	}

	// Check failures details
	if len(summary.Failures) != 2 {
		t.Fatalf("expected 2 failures, got %d", len(summary.Failures))
	}

	// Verify first failure (WARN level)
	if summary.Failures[0].Level != "warn" {
		t.Errorf("expected first failure Level = warn, got %s", summary.Failures[0].Level)
	}

	// Verify second failure (ERROR level)
	if summary.Failures[1].Level != "error" {
		t.Errorf("expected second failure Level = error, got %s", summary.Failures[1].Level)
	}
}
