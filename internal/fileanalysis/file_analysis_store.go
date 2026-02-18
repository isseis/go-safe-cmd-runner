package fileanalysis

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// HashFilePathGetter generates file paths from content hashes.
// This interface is defined locally to avoid import cycles with filevalidator.
// filevalidator.HybridHashFilePathGetter implements this interface implicitly
// by having the same method signature.
type HashFilePathGetter interface {
	GetHashFilePath(hashDir string, filePath common.ResolvedPath) (string, error)
}

const (
	// filePermission is the permission mode for analysis record files.
	filePermission = 0o600

	// dirPermission is the permission mode for analysis result directory.
	// 0o750 allows owner full access, group read/execute, others no access.
	dirPermission = 0o750
)

// Store manages unified file analysis record files containing both
// hash validation and syscall analysis data.
// Note: This type was renamed from FileAnalysisStore to avoid stuttering
// (fileanalysis.Store instead of fileanalysis.FileAnalysisStore).
type Store struct {
	analysisDir string
	pathGetter  HashFilePathGetter
}

// NewStore creates a new Store.
// If analysisDir does not exist, it will be created with mode 0o750.
// This simplifies operational workflows by eliminating the need for
// manual directory creation before running the record command.
//
// TOCTOU Note: There is a potential race condition between os.Lstat() and
// os.MkdirAll() where a symlink could be created in between. However, this
// risk is mitigated because:
// 1. Individual file I/O operations use safefileio which protects against symlink attacks
// 2. The analysisDir is typically under a trusted location controlled by the operator
// 3. An attacker with write access to the parent directory already has significant control
func NewStore(analysisDir string, pathGetter HashFilePathGetter) (*Store, error) {
	// Check if directory exists
	info, err := os.Lstat(analysisDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Create directory if it doesn't exist
			if err := os.MkdirAll(analysisDir, dirPermission); err != nil {
				return nil, fmt.Errorf("failed to create analysis result directory: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to access analysis result directory: %w", err)
		}
	} else if !info.IsDir() {
		// Path exists but is not a directory
		return nil, fmt.Errorf("%w: %s", ErrAnalysisDirNotDirectory, analysisDir)
	}

	return &Store{
		analysisDir: analysisDir,
		pathGetter:  pathGetter,
	}, nil
}

// Load loads the analysis record for the given file path.
// Returns ErrRecordNotFound if the analysis record file does not exist.
func (s *Store) Load(filePath string) (*Record, error) {
	recordPath, err := s.getRecordPath(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get analysis record path: %w", err)
	}

	data, err := safefileio.SafeReadFile(recordPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("failed to read analysis record file: %w", err)
	}

	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, &RecordCorruptedError{Path: recordPath, Cause: err}
	}

	// Validate schema version
	if record.SchemaVersion != CurrentSchemaVersion {
		return nil, &SchemaVersionMismatchError{
			Expected: CurrentSchemaVersion,
			Actual:   record.SchemaVersion,
		}
	}

	return &record, nil
}

// Save saves the analysis record for the given file path.
// This overwrites the entire record. Use Update for read-modify-write operations.
func (s *Store) Save(filePath string, record *Record) error {
	recordPath, err := s.getRecordPath(filePath)
	if err != nil {
		return fmt.Errorf("failed to get analysis record path: %w", err)
	}

	record.SchemaVersion = CurrentSchemaVersion
	record.FilePath = filePath
	record.UpdatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal analysis record: %w", err)
	}

	if err := safefileio.SafeWriteFileOverwrite(recordPath, data, filePermission); err != nil {
		return fmt.Errorf("failed to write analysis record file: %w", err)
	}

	return nil
}

// Update performs a read-modify-write operation on the analysis record.
// The updateFn receives the existing record (or a new empty one if not found)
// and should modify it in place.
//
// Error Handling:
//   - ErrRecordNotFound: creates a new record
//   - RecordCorruptedError: creates a new record (overwriting corrupted data)
//   - SchemaVersionMismatchError: returns error without overwriting
//     (preserves forward/backward compatibility until migration strategy is defined)
func (s *Store) Update(filePath string, updateFn func(*Record) error) error {
	// Try to load existing record
	record, err := s.Load(filePath)
	if err != nil {
		if errors.As(err, new(*SchemaVersionMismatchError)) {
			// Do not overwrite records with different schema versions.
			// This prevents accidental data loss when a record was created by a
			// newer version (forward compatibility) or uses an old schema that
			// requires migration.
			return fmt.Errorf("cannot update record: %w", err)
		}

		if errors.Is(err, ErrRecordNotFound) || errors.As(err, new(*RecordCorruptedError)) {
			// Create a new record if it's not found or if the existing one is corrupted.
			record = &Record{}
		} else {
			// For any other unknown error, fail safely.
			return fmt.Errorf("failed to load existing record: %w", err)
		}
	}

	// Apply update
	if err := updateFn(record); err != nil {
		return err
	}

	// Save updated record
	return s.Save(filePath, record)
}

// getRecordPath returns the analysis record file path for the given file.
func (s *Store) getRecordPath(filePath string) (string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	resolvedPath, err := common.NewResolvedPath(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to create resolved path: %w", err)
	}

	return s.pathGetter.GetHashFilePath(s.analysisDir, resolvedPath)
}
