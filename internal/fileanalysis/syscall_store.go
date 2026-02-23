package fileanalysis

import (
	"fmt"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// SyscallAnalysisResult represents the result of syscall analysis.
// This type mirrors elfanalyzer.SyscallAnalysisResult to avoid import cycles.
// Both types embed common.SyscallAnalysisResultCore, enabling direct struct copy
// for type conversion between these packages.
type SyscallAnalysisResult struct {
	// SyscallAnalysisResultCore contains the common fields shared with
	// elfanalyzer.SyscallAnalysisResult. Embedding ensures field-level
	// consistency between packages and enables direct struct copy for
	// type conversion.
	common.SyscallAnalysisResultCore
}

// SyscallAnalysisStore defines the interface for storing and loading syscall analysis results.
// This interface uses fileanalysis types to avoid import cycles with elfanalyzer.
// Used directly by cmd/record for saving/loading syscall analysis.
type SyscallAnalysisStore interface {
	// LoadSyscallAnalysis loads syscall analysis from storage.
	// `expectedHash` contains both the hash algorithm and the expected hash value.
	// Returns (result, nil) if found and hash matches.
	// Returns (nil, ErrRecordNotFound) if record not found.
	// Returns (nil, ErrHashMismatch) if hash mismatch.
	// Returns (nil, ErrNoSyscallAnalysis) if no syscall analysis data exists.
	// Returns (nil, error) on other errors (e.g., schema mismatch, corrupted record).
	LoadSyscallAnalysis(filePath string, expectedHash string) (*SyscallAnalysisResult, error)

	// SaveSyscallAnalysis saves the syscall analysis result.
	SaveSyscallAnalysis(filePath, fileHash string, result *SyscallAnalysisResult) error
}

// syscallAnalysisStore implements SyscallAnalysisStore.
// This is a concrete adapter backed by Store.
// The type is unexported to avoid confusion with the interface defined above.
type syscallAnalysisStore struct {
	store *Store
}

// NewSyscallAnalysisStore creates a new SyscallAnalysisStore
// backed by Store.
func NewSyscallAnalysisStore(store *Store) SyscallAnalysisStore {
	return &syscallAnalysisStore{store: store}
}

// SaveSyscallAnalysis saves the syscall analysis result.
// This updates only the syscall_analysis field, preserving other fields.
func (s *syscallAnalysisStore) SaveSyscallAnalysis(filePath, fileHash string, result *SyscallAnalysisResult) error {
	resolvedPath, err := common.NewResolvedPath(filePath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	return s.store.Update(resolvedPath, func(record *Record) error {
		record.ContentHash = fileHash
		// Copy the shared core fields directly via the embedded struct,
		// then set the fileanalysis-specific AnalyzedAt field.
		record.SyscallAnalysis = &SyscallAnalysisData{
			SyscallAnalysisResultCore: result.SyscallAnalysisResultCore,
			AnalyzedAt:                time.Now().UTC(),
		}
		return nil
	})
}

// LoadSyscallAnalysis loads the syscall analysis result.
// Returns (result, nil) if found and hash matches.
// Returns (nil, ErrRecordNotFound) if record not found.
// Returns (nil, ErrHashMismatch) if hash mismatch.
// Returns (nil, ErrNoSyscallAnalysis) if no syscall analysis data exists.
// Returns (nil, error) on other errors (e.g., schema mismatch, corrupted record).
func (s *syscallAnalysisStore) LoadSyscallAnalysis(filePath, expectedHash string) (*SyscallAnalysisResult, error) {
	resolvedPath, err := common.NewResolvedPath(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}
	record, err := s.store.Load(resolvedPath)
	if err != nil {
		return nil, err
	}

	// Check hash match
	if record.ContentHash != expectedHash {
		return nil, ErrHashMismatch
	}

	// Check if syscall analysis exists
	if record.SyscallAnalysis == nil {
		return nil, ErrNoSyscallAnalysis
	}

	// Convert SyscallAnalysisData to SyscallAnalysisResult.
	// Both types embed common.SyscallAnalysisResultCore, so the core fields
	// can be copied directly without field-by-field assignment.
	result := &SyscallAnalysisResult{
		SyscallAnalysisResultCore: record.SyscallAnalysis.SyscallAnalysisResultCore,
	}

	return result, nil
}
