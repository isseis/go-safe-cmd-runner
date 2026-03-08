package dynlibanalysis

import (
	"fmt"
	"strings"
)

// ErrLibraryNotResolved indicates that a DT_NEEDED library could not be resolved
// to a filesystem path through any of the available search methods.
type ErrLibraryNotResolved struct {
	SOName      string
	ParentPath  string
	SearchPaths []string // all paths that were tried
}

func (e *ErrLibraryNotResolved) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "failed to resolve dynamic library: %s\n", e.SOName)
	fmt.Fprintf(&sb, "  parent: %s\n", e.ParentPath)
	sb.WriteString("  searched paths:\n")
	for _, p := range e.SearchPaths {
		fmt.Fprintf(&sb, "    - %s\n", p)
	}
	return sb.String()
}

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
// but not present for an ELF binary.
type ErrDynLibDepsRequired struct {
	BinaryPath string
}

func (e *ErrDynLibDepsRequired) Error() string {
	return fmt.Sprintf("dynamic library dependencies not recorded for ELF binary: %s\n"+
		"  please re-run 'record' command",
		e.BinaryPath)
}
