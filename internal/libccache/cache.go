package libccache

import (
	"debug/elf"
	"encoding/json"
	"fmt"
	"os"
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
	cacheFilePath := m.cacheDir + "/" + encodedName

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
		return nil, ErrLibcFileNotAccessible
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

	// Write cache file.
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
	if err := os.WriteFile(cacheFilePath, cacheData, cacheFilePerm); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheWriteFailed, err)
	}

	return wrappers, nil
}
