package fileanalysis

import (
	"fmt"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// SyscallAnalysisResult represents the result of syscall analysis.
// This type mirrors elfanalyzer.SyscallAnalysisResult to avoid import cycles.
// The elfanalyzer package will define its own interface that refers to its types,
// and adapters in Phase 5 will handle type conversion.
type SyscallAnalysisResult struct {
	// Architecture is the ELF machine architecture that was analyzed (e.g., "x86_64").
	Architecture string

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
		record.SyscallAnalysis = &SyscallAnalysisData{
			Architecture:       result.Architecture,
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

	// Convert SyscallAnalysisData to SyscallAnalysisResult
	result := &SyscallAnalysisResult{
		Architecture:       record.SyscallAnalysis.Architecture,
		DetectedSyscalls:   record.SyscallAnalysis.DetectedSyscalls,
		HasUnknownSyscalls: record.SyscallAnalysis.HasUnknownSyscalls,
		HighRiskReasons:    record.SyscallAnalysis.HighRiskReasons,
		Summary:            record.SyscallAnalysis.Summary,
	}

	return result, nil
}
