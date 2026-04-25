package fileanalysis

import (
	"fmt"
	"sort"

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
	// Returns (nil, nil) if no syscall analysis data exists (e.g., not applicable, skipped, or none detected).
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

	// Group DetectedSyscalls by number, merging Occurrences
	groups := make(map[int]*common.SyscallInfo)
	var numberOrder []int
	seenNumber := make(map[int]bool)

	for _, info := range result.DetectedSyscalls {
		if !seenNumber[info.Number] {
			seenNumber[info.Number] = true
			numberOrder = append(numberOrder, info.Number)
		}
		if _, exists := groups[info.Number]; !exists {
			groups[info.Number] = &common.SyscallInfo{
				Number:      info.Number,
				Name:        info.Name,
				IsNetwork:   info.IsNetwork,
				Occurrences: make([]common.SyscallOccurrence, 0),
			}
		}
		groups[info.Number].Occurrences = append(groups[info.Number].Occurrences, info.Occurrences...)
	}

	// Sort each group's Occurrences by Location
	for _, group := range groups {
		sort.SliceStable(group.Occurrences, func(i, j int) bool {
			return group.Occurrences[i].Location < group.Occurrences[j].Location
		})
	}

	// Sort number groups: ascending order, with -1 at the end
	sort.SliceStable(numberOrder, func(i, j int) bool {
		ni, nj := numberOrder[i], numberOrder[j]
		if ni == -1 && nj == -1 {
			return false
		}
		if ni == -1 {
			return false
		}
		if nj == -1 {
			return true
		}
		return ni < nj
	})

	// Build result
	sorted := make([]common.SyscallInfo, 0, len(groups))
	for _, num := range numberOrder {
		sorted = append(sorted, *groups[num])
	}

	return s.store.Update(resolvedPath, func(record *Record) error {
		record.ContentHash = fileHash
		analysisData := &SyscallAnalysisData{
			SyscallAnalysisResultCore: result.SyscallAnalysisResultCore,
		}
		analysisData.DetectedSyscalls = sorted
		record.SyscallAnalysis = analysisData
		return nil
	})
}

// LoadSyscallAnalysis loads the syscall analysis result.
// Returns (result, nil) if found and hash matches.
// Returns (nil, ErrRecordNotFound) if record not found.
// Returns (nil, ErrHashMismatch) if hash mismatch.
// Returns (nil, nil) if no syscall analysis data exists (analyzed but none detected).
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
		return nil, nil
	}

	// Convert SyscallAnalysisData to SyscallAnalysisResult.
	// Both types embed common.SyscallAnalysisResultCore, so the core fields
	// can be copied directly without field-by-field assignment.
	result := &SyscallAnalysisResult{
		SyscallAnalysisResultCore: record.SyscallAnalysis.SyscallAnalysisResultCore,
	}

	return result, nil
}
