package libccache

import (
	"debug/elf"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator/pathencoding"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

const (
	cacheDirPerm  = 0o750
	cacheFilePerm = 0o600
)

// LibcCacheManager manages the read and write of libc analysis cache files.
//
//nolint:revive // LibcCacheManager is intentional: callers import as libccache.LibcCacheManager
type LibcCacheManager struct {
	cacheDir string
	fs       safefileio.FileSystem
	analyzer *LibcWrapperAnalyzer
	pathEnc  *pathencoding.SubstitutionHashEscape
}

// NewLibcCacheManager creates a new LibcCacheManager.
// cacheDir is the path to the cache directory (created automatically if it does not exist).
func NewLibcCacheManager(
	cacheDir string,
	fs safefileio.FileSystem,
	analyzer *LibcWrapperAnalyzer,
) (*LibcCacheManager, error) {
	if err := os.MkdirAll(cacheDir, cacheDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create cache directory %s: %w", cacheDir, err)
	}
	return &LibcCacheManager{
		cacheDir: cacheDir,
		fs:       fs,
		analyzer: analyzer,
		pathEnc:  pathencoding.NewSubstitutionHashEscape(),
	}, nil
}

// GetOrCreate returns cached wrappers, or analyzes libc and creates the cache on miss.
// libcPath is the normalized real path of the libc file.
// libcHash is the hash in "sha256:<hex>" format.
// The libc file is opened only on cache miss.
func (m *LibcCacheManager) GetOrCreate(libcPath, libcHash string) ([]WrapperEntry, error) {
	encodedName, err := m.pathEnc.Encode(libcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to encode libc path: %w", err)
	}
	cacheFilePath := filepath.Join(m.cacheDir, encodedName)

	// Try to load and validate the existing cache.
	if data, err := os.ReadFile(cacheFilePath); err == nil { //nolint:nestif,gosec // G304: cacheFilePath = cacheDir + pathEnc.Encode(libcPath), both trusted
		var cache LibcCacheFile
		if jsonErr := json.Unmarshal(data, &cache); jsonErr == nil {
			if cache.SchemaVersion == LibcCacheSchemaVersion && cache.LibHash == libcHash {
				return cache.SyscallWrappers, nil
			}
		}
	}

	// Cache MISS: open and analyze the libc file.
	libcFile, err := m.fs.SafeOpenFile(libcPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLibcFileNotAccessible, err)
	}

	elfFile, err := elf.NewFile(libcFile)
	if err != nil {
		_ = libcFile.Close() // manual close on elf.NewFile failure; error not actionable in error path
		return nil, err
	}
	defer elfFile.Close() //nolint:errcheck // on success, elfFile.Close() also closes libcFile; error not actionable

	wrappers, err := m.analyzer.Analyze(elfFile)
	if err != nil {
		return nil, err
	}

	// Write cache file atomically: write to a temp file, then rename.
	cache := LibcCacheFile{
		SchemaVersion:   LibcCacheSchemaVersion,
		LibPath:         libcPath,
		LibHash:         libcHash,
		AnalyzedAt:      time.Now().UTC().Format(time.RFC3339),
		SyscallWrappers: wrappers,
	}
	cacheData, err := json.Marshal(cache)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheWriteFailed, err)
	}
	if err := writeFileAtomic(cacheFilePath, cacheData, cacheFilePerm); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheWriteFailed, err)
	}

	return wrappers, nil
}

// writeFileAtomic writes data to path atomically by writing to a temp file in the
// same directory and then renaming it. This prevents partial reads by concurrent
// processes reading the cache file while it is being written.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".cache-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name() // tmpPath is returned by os.CreateTemp; not user-controlled
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath) //nolint:gosec // G703: tmpPath from os.CreateTemp, not user-controlled
		return err
	}
	if err := tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath) //nolint:gosec // G703: tmpPath from os.CreateTemp, not user-controlled
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath) //nolint:gosec // G703: tmpPath from os.CreateTemp, not user-controlled
		return err
	}
	return os.Rename(tmpPath, path) //nolint:gosec // G703: tmpPath from os.CreateTemp, not user-controlled
}
