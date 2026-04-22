package fileanalysis

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// NetworkSymbolStore defines the interface for loading network symbol analysis results.
// This interface uses fileanalysis types to avoid import cycles with the security package.
type NetworkSymbolStore interface {
	// LoadNetworkSymbolAnalysis loads the cached network symbol analysis for the given file.
	// Returns (data, nil) if found and hash matches.
	// Returns (nil, ErrRecordNotFound) if record not found.
	// Returns (nil, ErrHashMismatch) if hash does not match.
	// Returns (nil, nil) if no network symbol analysis exists (e.g., not applicable, skipped, or none detected).
	// Returns (nil, error) on other errors.
	LoadNetworkSymbolAnalysis(filePath string, contentHash string) (*SymbolAnalysisData, error)
}

// networkSymbolStore implements NetworkSymbolStore backed by Store.
type networkSymbolStore struct {
	store *Store
}

// NewNetworkSymbolStore creates a new NetworkSymbolStore backed by Store.
func NewNetworkSymbolStore(store *Store) NetworkSymbolStore {
	return &networkSymbolStore{store: store}
}

// LoadNetworkSymbolAnalysis loads the cached network symbol analysis for the given file.
// Returns (data, nil) if found and hash matches.
// Returns (nil, ErrRecordNotFound) if record not found.
// Returns (nil, ErrHashMismatch) if hash does not match.
// Returns (nil, nil) if no network symbol analysis exists (analyzed but none detected).
// Returns (nil, error) on other errors (e.g., schema mismatch).
func (s *networkSymbolStore) LoadNetworkSymbolAnalysis(filePath string, contentHash string) (*SymbolAnalysisData, error) {
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

	if record.SymbolAnalysis == nil {
		return nil, nil
	}

	return record.SymbolAnalysis, nil
}
