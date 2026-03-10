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
	// Returns (nil, ErrNoNetworkSymbolAnalysis) if no network symbol analysis exists.
	// Returns (nil, error) on other errors.
	LoadNetworkSymbolAnalysis(filePath string, expectedHash string) (*NetworkSymbolAnalysisData, error)
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
// Returns (nil, ErrNoNetworkSymbolAnalysis) if no network symbol analysis exists.
// Returns (nil, error) on other errors (e.g., schema mismatch).
func (s *networkSymbolStore) LoadNetworkSymbolAnalysis(filePath string, expectedHash string) (*NetworkSymbolAnalysisData, error) {
	resolvedPath, err := common.NewResolvedPath(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	record, err := s.store.Load(resolvedPath)
	if err != nil {
		return nil, err
	}

	if record.ContentHash != expectedHash {
		return nil, ErrHashMismatch
	}

	if record.NetworkSymbolAnalysis == nil {
		return nil, ErrNoNetworkSymbolAnalysis
	}

	return record.NetworkSymbolAnalysis, nil
}
