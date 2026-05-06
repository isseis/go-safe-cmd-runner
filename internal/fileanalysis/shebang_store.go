package fileanalysis

import (
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// ShebangInterpreterStore provides the interpreter binary path and content hash
// for a shebang script, enabling the runner to follow the shebang chain for
// risk assessment without re-analyzing the interpreter binary.
type ShebangInterpreterStore interface {
	// LoadInterpreterAnalysisPath returns the effective interpreter binary path
	// and its content hash for the shebang script at scriptPath.
	//
	// scriptContentHash is validated against the stored record to detect
	// script file changes (ErrHashMismatch).
	//
	// Returns ("", "", nil) when:
	//   - The script has no ShebangInterpreter (not a shebang script)
	//
	// Returns error when:
	//   - The script's record cannot be loaded (non-ErrRecordNotFound errors)
	//   - scriptContentHash does not match stored hash (ErrHashMismatch)
	//   - The interpreter's record is not found (ErrInterpreterRecordMissing)
	//   - The interpreter's content hash is empty (ErrInterpreterRecordMissing)
	//   - The interpreter's record cannot be loaded (non-ErrRecordNotFound errors)
	LoadInterpreterAnalysisPath(scriptPath, scriptContentHash string) (interpPath, interpContentHash string, err error)
}

// shebangInterpreterStore implements ShebangInterpreterStore using the file
// analysis record store.
type shebangInterpreterStore struct {
	store *Store
}

// NewShebangInterpreterStore creates a ShebangInterpreterStore backed by store.
func NewShebangInterpreterStore(store *Store) ShebangInterpreterStore {
	return &shebangInterpreterStore{store: store}
}

// LoadInterpreterAnalysisPath loads the analysis record for a shebang script and
// returns the effective interpreter path and its content hash.
func (s *shebangInterpreterStore) LoadInterpreterAnalysisPath(scriptPath, scriptContentHash string) (interpPath, interpContentHash string, err error) {
	scriptTarget, err := common.NewResolvedPath(scriptPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve script path: %w", err)
	}

	scriptRecord, err := s.store.Load(scriptTarget)
	if err != nil {
		if errors.Is(err, ErrRecordNotFound) {
			return "", "", ErrRecordNotFound
		}
		return "", "", err
	}

	if scriptRecord.ContentHash != scriptContentHash {
		return "", "", ErrHashMismatch
	}

	if scriptRecord.ShebangInterpreter == nil {
		return "", "", nil
	}

	si := scriptRecord.ShebangInterpreter
	if si.ResolvedPath != "" {
		interpPath = si.ResolvedPath
	} else {
		interpPath = si.InterpreterPath
	}

	interpTarget, err := common.NewResolvedPath(interpPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve interpreter path: %w", err)
	}

	interpRecord, err := s.store.Load(interpTarget)
	if err != nil {
		if errors.Is(err, ErrRecordNotFound) {
			return "", "", fmt.Errorf("interpreter record not found: %w", ErrInterpreterRecordMissing)
		}
		return "", "", err
	}

	if interpRecord.ContentHash == "" {
		return "", "", fmt.Errorf("interpreter record has empty content hash: %w", ErrInterpreterRecordMissing)
	}

	return interpPath, interpRecord.ContentHash, nil
}
