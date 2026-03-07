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

// ErrLibraryPathMismatch indicates that a library resolved to a different path
// than what was recorded (Stage 2 verification failure).
type ErrLibraryPathMismatch struct {
	SOName       string
	ParentPath   string
	RecordedPath string
	ResolvedPath string
}

func (e *ErrLibraryPathMismatch) Error() string {
	return fmt.Sprintf("dynamic library path mismatch: %s\n"+
		"  recorded path: %s\n"+
		"  resolved path: %s\n"+
		"  parent: %s\n"+
		"  cause: LD_LIBRARY_PATH may have been modified\n"+
		"  please re-run 'record' command",
		e.SOName, e.RecordedPath, e.ResolvedPath, e.ParentPath)
}

// ErrEmptyLibraryPath indicates that a LibEntry has an empty path,
// which should never happen in valid records (defensive check).
type ErrEmptyLibraryPath struct {
	SOName     string
	ParentPath string
}

func (e *ErrEmptyLibraryPath) Error() string {
	return fmt.Sprintf("incomplete record: empty path for library %s (parent: %s)\n"+
		"  please re-run 'record' command",
		e.SOName, e.ParentPath)
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
