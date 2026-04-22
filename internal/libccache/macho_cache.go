package libccache

import (
	"bytes"
	"debug/macho"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator/pathencoding"
)

// MachoLibSystemCacheManager manages read and write of libSystem analysis cache files.
// Uses the same LibcCacheFile schema and cache directory as LibcCacheManager.
//
//nolint:revive // MachoLibSystemCacheManager is intentional: callers import as libccache.MachoLibSystemCacheManager
type MachoLibSystemCacheManager struct {
	cacheDir string
	analyzer *MachoLibSystemAnalyzer
	pathEnc  *pathencoding.SubstitutionHashEscape
}

// NewMachoLibSystemCacheManager creates a new MachoLibSystemCacheManager.
// cacheDir is the path to the cache directory (created automatically if it does not exist).
func NewMachoLibSystemCacheManager(cacheDir string) (*MachoLibSystemCacheManager, error) {
	if err := os.MkdirAll(cacheDir, cacheDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create cache directory %s: %w", cacheDir, err)
	}
	return &MachoLibSystemCacheManager{
		cacheDir: cacheDir,
		analyzer: &MachoLibSystemAnalyzer{},
		pathEnc:  pathencoding.NewSubstitutionHashEscape(),
	}, nil
}

// GetOrCreate returns cached wrappers, or analyzes libsystem_kernel and creates cache on miss.
// libPath is used for cache file naming and the lib_path field (install name or real file path).
// libHash is the "sha256:<hex>" validity hash.
// getData is a callback that returns Mach-O bytes on cache miss only.
func (m *MachoLibSystemCacheManager) GetOrCreate(
	libPath, libHash string,
	getData func() ([]byte, error),
) ([]WrapperEntry, error) {
	encodedName, err := m.pathEnc.Encode(libPath)
	if err != nil {
		return nil, fmt.Errorf("failed to encode libsystem path: %w", err)
	}
	cacheFilePath := filepath.Join(m.cacheDir, encodedName)

	// Return cached wrappers when the schema version and lib hash both match.
	if data, readErr := os.ReadFile(cacheFilePath); readErr == nil { //nolint:nestif,gosec // #nosec G304 -- cacheFilePath = cacheDir + pathEnc.Encode(libPath), both trusted
		var cache LibcCacheFile
		if jsonErr := json.Unmarshal(data, &cache); jsonErr == nil {
			if cache.SchemaVersion == LibcCacheSchemaVersion && cache.LibHash == libHash {
				return cache.SyscallWrappers, nil
			}
		}
	}

	// Cache miss: obtain Mach-O bytes through getData() and analyze them.
	machoBytes, err := getData()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLibcFileNotAccessible, err)
	}

	machoFile, err := macho.NewFile(bytes.NewReader(machoBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Mach-O bytes: %w", err)
	}
	defer func() { _ = machoFile.Close() }()

	wrappers, err := m.analyzer.Analyze(machoFile)
	if err != nil {
		return nil, err
	}

	// Write the cache file atomically, same as the ELF path.
	cache := LibcCacheFile{
		SchemaVersion:   LibcCacheSchemaVersion,
		LibPath:         libPath,
		LibHash:         libHash,
		AnalyzedAt:      time.Now().UTC().Format(time.RFC3339),
		SyscallWrappers: wrappers,
	}
	cacheData, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheWriteFailed, err)
	}
	if err := writeFileAtomic(cacheFilePath, cacheData, cacheFilePerm); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheWriteFailed, err)
	}

	return wrappers, nil
}
