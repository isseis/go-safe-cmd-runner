// Package dynlibanalysisstore provides persistent storage for dynamic library
// analysis results. Results are stored per library, keyed by path and hash,
// and shared across record runs to avoid redundant analysis.
package dynlibanalysisstore

import "errors"

// ErrAnalysisNotFound is returned when the analysis result is not found or is invalid.
// This includes: file not found, schema_version mismatch, lib_hash mismatch, and parse errors.
var ErrAnalysisNotFound = errors.New("dynlibanalysisstore: analysis not found")
