package dynamicanalysis

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator/pathencoding"
)

const (
	storeDirPerm  = 0o755
	storeFilePerm = 0o644
)

// store manages storage and retrieval of dynamic library analysis results on
// disk, keyed by library path and hash.
type store struct {
	storeDir string
	analyzer Analyzer
	pathEnc  *pathencoding.SubstitutionHashEscape
}

// New creates a new Store backed by storeDir.
// storeDir is created automatically if it does not exist.
// Pass a nil analyzer only when LoadOrAnalyzeAndStore will not be called (e.g., runner mode).
func New(storeDir string, analyzer Analyzer) (Store, error) {
	if err := os.MkdirAll(storeDir, storeDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create store directory %s: %w", storeDir, err)
	}
	return &store{
		storeDir: storeDir,
		analyzer: analyzer,
		pathEnc:  pathencoding.NewSubstitutionHashEscape(),
	}, nil
}

// LoadAnalysis retrieves stored analysis for the given library.
// Returns ErrAnalysisNotFound if no valid analysis exists.
func (s *store) LoadAnalysis(libPath, libHash string) (*Result, error) {
	return s.load(libPath, libHash)
}

// LoadOrAnalyzeAndStore retrieves existing analysis for the given library.
// On a miss it runs a fresh analysis, persists the result, and returns it.
func (s *store) LoadOrAnalyzeAndStore(libPath, libHash string) (*Result, error) {
	result, loadErr := s.load(libPath, libHash)
	if loadErr == nil {
		return result, nil
	}
	if !errors.Is(loadErr, ErrAnalysisNotFound) {
		return nil, loadErr
	}

	// Analysis not found: run fresh analysis.
	result, err := s.analyzer.AnalyzeLibrary(libPath)
	if err != nil {
		return nil, err
	}

	// Save result to disk; failure is non-fatal but surfaced as a warning.
	if saveErr := s.saveResult(libPath, libHash, result); saveErr != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("failed to save analysis result for %s: %v", libPath, saveErr))
	}

	return result, nil
}

// load reads and validates the stored analysis for the given library.
// Returns ErrAnalysisNotFound for all cases where the result cannot be reused
// (file not found, parse error, schema mismatch, hash mismatch).
func (s *store) load(libPath, libHash string) (*Result, error) {
	storeFilePath, err := s.storeFilePath(libPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute store file path: %w", err)
	}

	data, err := os.ReadFile(storeFilePath) //nolint:gosec // G304: storeFilePath = storeDir + pathEnc.Encode(libPath), both trusted
	if err != nil {
		return nil, ErrAnalysisNotFound
	}

	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, ErrAnalysisNotFound
	}

	if f.SchemaVersion != SchemaVersion || f.LibHash != libHash {
		return nil, ErrAnalysisNotFound
	}

	return &Result{
		SyscallAnalysis:    f.SyscallAnalysis,
		SymbolAnalysis:     f.SymbolAnalysis,
		DynamicLoadSymbols: f.DynamicLoadSymbols,
	}, nil
}

// saveResult writes the analysis result to disk atomically.
func (s *store) saveResult(libPath, libHash string, result *Result) error {
	storeFilePath, err := s.storeFilePath(libPath)
	if err != nil {
		return fmt.Errorf("failed to compute store file path: %w", err)
	}

	f := File{
		SchemaVersion:      SchemaVersion,
		LibPath:            libPath,
		LibHash:            libHash,
		SyscallAnalysis:    result.SyscallAnalysis,
		SymbolAnalysis:     result.SymbolAnalysis,
		DynamicLoadSymbols: result.DynamicLoadSymbols,
	}

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal analysis file: %w", err)
	}

	return writeFileAtomic(storeFilePath, data, storeFilePerm)
}

// storeFilePath returns the on-disk path for the given library path.
func (s *store) storeFilePath(libPath string) (string, error) {
	encodedName, err := s.pathEnc.Encode(libPath)
	if err != nil {
		return "", fmt.Errorf("failed to encode library path: %w", err)
	}
	return filepath.Join(s.storeDir, encodedName), nil
}

// writeFileAtomic writes data to path atomically by writing to a temp file in the
// same directory and then renaming it. This prevents partial reads by concurrent
// processes reading the store file while it is being written.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".store-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name() // tmpPath is returned by os.CreateTemp; not user-controlled

	// cleanup closes and removes the temporary file, joining any errors with the primary error.
	cleanup := func(primary error) error {
		closeErr := tmpFile.Close()
		removeErr := os.Remove(tmpPath) //nolint:gosec // G304: tmpPath from os.CreateTemp, not user-controlled
		return errors.Join(primary, closeErr, removeErr)
	}

	if _, err := tmpFile.Write(data); err != nil {
		return cleanup(err)
	}
	if err := tmpFile.Chmod(perm); err != nil {
		return cleanup(err)
	}
	if err := tmpFile.Close(); err != nil {
		removeErr := os.Remove(tmpPath) //nolint:gosec // G304: tmpPath from os.CreateTemp, not user-controlled
		return errors.Join(err, removeErr)
	}
	return os.Rename(tmpPath, path) //nolint:gosec // G304: tmpPath from os.CreateTemp, not user-controlled
}
