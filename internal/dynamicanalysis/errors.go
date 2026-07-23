// Package dynamicanalysis provides persistent storage for dynamic library
// analysis results. Results are stored per library, keyed by path and hash,
// and shared across record runs to avoid redundant analysis.
package dynamicanalysis

import "errors"

// StoreSubDir is the subdirectory name used within the hash directory to store
// dynamic library analysis results.
const StoreSubDir = "dynlib-analysis"

// ErrAnalysisNotFound is returned when the analysis result is not found or is invalid.
// This includes: file not found, schema_version mismatch, lib_hash mismatch, and parse errors.
var ErrAnalysisNotFound = errors.New("dynamicanalysis: analysis not found")

// ErrLibraryHashKeyMismatch is returned by LoadOrAnalyzeAndStore when the
// actual content hash of the analyzed library does not match libHash, the
// hash key the caller expects the analysis to correspond to. This indicates
// the file at libPath changed between when libHash was determined and when
// it was analyzed; the result is not persisted in this case (fail-closed).
var ErrLibraryHashKeyMismatch = errors.New("dynamicanalysis: library analysis hash does not match recorded hash key")
