// Package fileanalysis provides file analysis storage for syscall analysis results.
package fileanalysis

import (
	"errors"
	"fmt"
)

// Static errors
var (
	// ErrRecordNotFound indicates the analysis record file does not exist.
	ErrRecordNotFound = errors.New("analysis record file not found")

	// ErrAnalysisDirNotDirectory indicates the analysis result path is not a directory.
	ErrAnalysisDirNotDirectory = errors.New("analysis result path is not a directory")

	// ErrHashMismatch indicates the file content hash does not match the expected hash.
	ErrHashMismatch = errors.New("file content hash mismatch")

	// ErrInterpreterRecordMissing indicates that the shebang interpreter's analysis
	// record is absent or incomplete (empty content hash). This prevents risk
	// assessment from proceeding, so the runner must abort the command group.
	ErrInterpreterRecordMissing = errors.New("shebang interpreter analysis record missing or incomplete")
)

// SchemaVersionMismatchError indicates analysis record schema version mismatch.
type SchemaVersionMismatchError struct {
	Expected int
	Actual   int
}

func (e *SchemaVersionMismatchError) Error() string {
	return fmt.Sprintf("schema version mismatch: expected %d, got %d", e.Expected, e.Actual)
}

// RecordCorruptedError indicates analysis record file is corrupted.
type RecordCorruptedError struct {
	Path  string
	Cause error
}

func (e *RecordCorruptedError) Error() string {
	return fmt.Sprintf("analysis record file corrupted at %s: %v", e.Path, e.Cause)
}

func (e *RecordCorruptedError) Unwrap() error {
	return e.Cause
}
