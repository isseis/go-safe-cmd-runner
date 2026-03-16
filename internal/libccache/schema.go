package libccache

// LibcCacheSchemaVersion is the current schema version for libc cache files.
// Increment when making backward-incompatible schema changes.
const LibcCacheSchemaVersion = 1

// LibcCacheFile is the JSON schema for a libc cache file.
//
//nolint:revive // LibcCacheFile is intentional: callers import as libccache.LibcCacheFile
type LibcCacheFile struct {
	SchemaVersion   int            `json:"schema_version"`
	LibPath         string         `json:"lib_path"`
	LibHash         string         `json:"lib_hash"`
	AnalyzedAt      string         `json:"analyzed_at"` // RFC3339UTC format
	SyscallWrappers []WrapperEntry `json:"syscall_wrappers"`
}

// WrapperEntry represents a single syscall wrapper function in libc.
type WrapperEntry struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}
