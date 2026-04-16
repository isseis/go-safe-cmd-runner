// Package machodylib provides Mach-O dynamic library dependency analysis
// for LC_LOAD_DYLIB integrity verification.
//
// At record time (MachODynLibAnalyzer.Analyze), it resolves all LC_LOAD_DYLIB
// and LC_LOAD_WEAK_DYLIB entries recursively using BFS, computes SHA256 hashes
// of the resolved libraries, and returns a snapshot of the dependency tree.
//
// At verify time (HasDynamicLibDeps), it checks whether a Mach-O binary has
// any non-dyld-shared-cache dynamic library dependencies, which determines
// whether DynLibDeps should have been recorded.
//
// dyld shared cache libraries (macOS system libraries that live in the shared
// cache rather than as individual files on disk) are identified by
// IsDyldSharedCacheLib and excluded from hash verification.
package machodylib
