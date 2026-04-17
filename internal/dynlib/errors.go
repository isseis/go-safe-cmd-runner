// Package dynlib provides error types shared between ELF and Mach-O dynamic
// library analysis packages (elfdynlib and machodylib).
//
// These error types are format-independent and represent failure modes that
// occur during dependency resolution and verification regardless of whether
// the binary is an ELF or Mach-O file.
package dynlib

import "fmt"

// ErrRecursionDepthExceeded indicates that dependency resolution exceeded the
// maximum allowed depth. This typically indicates an abnormal library configuration
// or a missed circular dependency.
type ErrRecursionDepthExceeded struct {
	Depth    int
	MaxDepth int
	SOName   string
}

func (e *ErrRecursionDepthExceeded) Error() string {
	return fmt.Sprintf("dependency resolution depth exceeded: %s at depth %d (max %d)",
		e.SOName, e.Depth, e.MaxDepth)
}

// ErrLibraryHashMismatch indicates that a library's hash does not match the recorded value
// (Stage 1 verification failure).
type ErrLibraryHashMismatch struct {
	SOName       string
	Path         string
	ExpectedHash string
	ActualHash   string
}

func (e *ErrLibraryHashMismatch) Error() string {
	return fmt.Sprintf("dynamic library hash mismatch: %s\n"+
		"  path: %s\n"+
		"  expected hash: %s\n"+
		"  actual hash: %s\n"+
		"  please re-run 'record' command",
		e.SOName, e.Path, e.ExpectedHash, e.ActualHash)
}

// ErrEmptyLibraryPath indicates that a LibEntry has an empty path,
// which should never happen in valid records (defensive check).
type ErrEmptyLibraryPath struct {
	SOName string
}

func (e *ErrEmptyLibraryPath) Error() string {
	return fmt.Sprintf("incomplete record: empty path for library %s\n"+
		"  please re-run 'record' command",
		e.SOName)
}

// ErrDynLibDepsRequired indicates that a DynLibDeps record is required
// but not present for a binary.
type ErrDynLibDepsRequired struct {
	BinaryPath string
}

func (e *ErrDynLibDepsRequired) Error() string {
	return fmt.Sprintf("dynamic library dependencies not recorded for binary: %s\n"+
		"  please re-run 'record' command",
		e.BinaryPath)
}
