package fileanalysis

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// DynLibDepsStore defines the interface for loading dynamic library dependency records.
// Used by the runner to retrieve the DynLibDeps list recorded at record time, so that
// per-library analysis results can be looked up from the dynlibanalysisstore.
type DynLibDepsStore interface {
	// LoadDynLibDeps loads the dynamic library dependencies for the given file.
	// Returns (deps, nil) if found and hash matches.
	// Returns (nil, ErrRecordNotFound) if record not found.
	// Returns (nil, ErrHashMismatch) if hash does not match.
	// Returns (nil, nil) if no dyn lib dep data exists (e.g., static binary with no deps).
	// Returns (nil, error) on other errors (e.g., schema mismatch, corrupted record).
	LoadDynLibDeps(filePath string, contentHash string) ([]LibEntry, error)
}

// dynLibDepsStore implements DynLibDepsStore backed by Store.
type dynLibDepsStore struct {
	store *Store
}

// NewDynLibDepsStore creates a new DynLibDepsStore backed by Store.
func NewDynLibDepsStore(store *Store) DynLibDepsStore {
	return &dynLibDepsStore{store: store}
}

// LoadDynLibDeps loads the dynamic library dependencies for the given file.
func (s *dynLibDepsStore) LoadDynLibDeps(filePath string, contentHash string) ([]LibEntry, error) {
	resolvedPath, err := common.NewResolvedPath(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	record, err := s.store.Load(resolvedPath)
	if err != nil {
		return nil, err
	}

	if record.ContentHash != contentHash {
		return nil, ErrHashMismatch
	}

	if len(record.DynLibDeps) == 0 {
		return nil, nil
	}

	return record.DynLibDeps, nil
}
