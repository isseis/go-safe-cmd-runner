package libccache

// LibcCacheSchemaVersion is the current schema version for libc cache files.
// Increment when making backward-incompatible schema changes.
const LibcCacheSchemaVersion = 1

// SourceLibcSymbolImport is the value of SyscallInfo.Source for syscalls detected
// via libc import symbol matching.
const SourceLibcSymbolImport = "libc_symbol_import"

// SourceLibsystemSymbolImport is the value of SyscallInfo.Source for syscalls
// detected via libSystem import symbol matching.
const SourceLibsystemSymbolImport = "libsystem_symbol_import"

// DeterminationMethodLibCacheMatch indicates the syscall was determined via
// libSystem function-level analysis cache matching.
const DeterminationMethodLibCacheMatch = "lib_cache_match"

// DeterminationMethodSymbolNameMatch indicates the syscall was determined via
// symbol name-only matching (fallback path).
const DeterminationMethodSymbolNameMatch = "symbol_name_match"

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
// Number is guaranteed to be >= 0; validateInfos rejects entries with Number < 0
// before any WrapperEntry is constructed.
type WrapperEntry struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}
