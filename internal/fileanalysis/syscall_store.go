package fileanalysis

import (
	"errors"
	"time"
)

// SyscallAnalysisResult represents the result of syscall analysis.
// This type mirrors elfanalyzer.SyscallAnalysisResult to avoid import cycles.
// The elfanalyzer package will define its own interface that refers to its types,
// and adapters in Phase 5 will handle type conversion.
type SyscallAnalysisResult struct {
	// DetectedSyscalls contains all detected syscall events with their numbers.
	DetectedSyscalls []SyscallInfo

	// HasUnknownSyscalls indicates whether any syscall number could not be determined.
	HasUnknownSyscalls bool

	// HighRiskReasons explains why the analysis resulted in high risk, if applicable.
	HighRiskReasons []string

	// Summary provides aggregated information about the analysis.
	Summary SyscallSummary
}

// SyscallAnalysisStore defines the interface for storing and loading syscall analysis results.
// This interface uses fileanalysis types to avoid import cycles with elfanalyzer.
// Adapters in elfanalyzer package (Phase 5) will convert between types.
type SyscallAnalysisStore interface {
	// LoadSyscallAnalysis loads syscall analysis from storage.
	// `expectedHash` contains both the hash algorithm and the expected hash value.
	// Returns (result, true, nil) if found and hash matches.
	// Returns (nil, false, nil) if not found or hash mismatch.
	// Returns (nil, false, error) on other errors.
	LoadSyscallAnalysis(filePath string, expectedHash string) (*SyscallAnalysisResult, bool, error)

	// SaveSyscallAnalysis saves the syscall analysis result.
	SaveSyscallAnalysis(filePath, fileHash string, result *SyscallAnalysisResult) error
}

// syscallAnalysisStoreImpl implements SyscallAnalysisStore.
// This is a concrete adapter backed by Store.
// The type is unexported to avoid confusion with the interface defined above.
type syscallAnalysisStoreImpl struct {
	store *Store
}

// NewSyscallAnalysisStore creates a new SyscallAnalysisStore
// backed by Store.
func NewSyscallAnalysisStore(store *Store) SyscallAnalysisStore {
	return &syscallAnalysisStoreImpl{store: store}
}

// SaveSyscallAnalysis saves the syscall analysis result.
// This updates only the syscall_analysis field, preserving other fields.
func (s *syscallAnalysisStoreImpl) SaveSyscallAnalysis(filePath, fileHash string, result *SyscallAnalysisResult) error {
	return s.store.Update(filePath, func(record *Record) error {
		record.ContentHash = fileHash
		record.SyscallAnalysis = &SyscallAnalysisData{
			Architecture:       "x86_64",
			AnalyzedAt:         time.Now().UTC(),
			DetectedSyscalls:   result.DetectedSyscalls,
			HasUnknownSyscalls: result.HasUnknownSyscalls,
			HighRiskReasons:    result.HighRiskReasons,
			Summary:            result.Summary,
		}
		return nil
	})
}

// LoadSyscallAnalysis loads the syscall analysis result.
// Returns (result, true, nil) if found and hash matches.
// Returns (nil, false, nil) if not found or hash mismatch.
// Returns (nil, false, error) on other errors.
func (s *syscallAnalysisStoreImpl) LoadSyscallAnalysis(filePath, expectedHash string) (*SyscallAnalysisResult, bool, error) {
	record, err := s.store.Load(filePath)
	if err != nil {
		if errors.Is(err, ErrRecordNotFound) {
			return nil, false, nil
		}
		// For schema mismatch or corrupted records, return not found
		var schemaErr *SchemaVersionMismatchError
		if errors.As(err, &schemaErr) {
			return nil, false, nil
		}
		var corruptedErr *RecordCorruptedError
		if errors.As(err, &corruptedErr) {
			return nil, false, nil
		}
		return nil, false, err
	}

	// Check hash match
	if record.ContentHash != expectedHash {
		return nil, false, nil
	}

	// Check if syscall analysis exists
	if record.SyscallAnalysis == nil {
		return nil, false, nil
	}

	// Convert SyscallAnalysisData to SyscallAnalysisResult
	result := &SyscallAnalysisResult{
		DetectedSyscalls:   record.SyscallAnalysis.DetectedSyscalls,
		HasUnknownSyscalls: record.SyscallAnalysis.HasUnknownSyscalls,
		HighRiskReasons:    record.SyscallAnalysis.HighRiskReasons,
		Summary:            record.SyscallAnalysis.Summary,
	}

	return result, true, nil
}
