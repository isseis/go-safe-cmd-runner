package libccache

import "errors"

var (
	// ErrLibcFileNotAccessible indicates that the libc file could not be read.
	ErrLibcFileNotAccessible = errors.New("libc file not accessible")

	// ErrExportSymbolsFailed indicates that export symbol retrieval from libc failed.
	ErrExportSymbolsFailed = errors.New("failed to get export symbols from libc")

	// ErrCacheWriteFailed indicates that writing the libc cache file failed.
	ErrCacheWriteFailed = errors.New("failed to write libc cache file")
)

// Unsupported architecture errors (elfanalyzer.UnsupportedArchitectureError) propagate
// without wrapping. Callers should detect them with errors.As:
//
//	var archErr *elfanalyzer.UnsupportedArchitectureError
//	if errors.As(err, &archErr) { ... }
