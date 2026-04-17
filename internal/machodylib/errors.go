package machodylib

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotMachO is returned by openMachO when the file is not a valid Mach-O or
// Fat binary. Callers use errors.Is to distinguish "not Mach-O" (skip silently)
// from real failures such as I/O errors or ErrNoMatchingSlice.
var ErrNotMachO = errors.New("not a Mach-O file")

// ErrLibraryNotResolved indicates that an LC_LOAD_DYLIB install name could not
// be resolved to a filesystem path through any of the available search methods.
type ErrLibraryNotResolved struct {
	InstallName string
	LoaderPath  string
	Tried       []string // all paths that were tried
}

func (e *ErrLibraryNotResolved) Error() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "failed to resolve dynamic library: %s\n", e.InstallName)
	fmt.Fprintf(&sb, "  loader: %s\n", e.LoaderPath)

	if len(e.Tried) > 0 {
		sb.WriteString("  tried:\n")
		for _, p := range e.Tried {
			fmt.Fprintf(&sb, "    - %s (not found)\n", p)
		}
	}

	return sb.String()
}

// ErrUnknownAtToken indicates that an unrecognized @ prefix token was found
// in an install name. Only @executable_path, @loader_path, and @rpath are
// supported.
type ErrUnknownAtToken struct {
	InstallName string
	Token       string
}

func (e *ErrUnknownAtToken) Error() string {
	return fmt.Sprintf("unknown @ token in install name: %s (token: %s)",
		e.InstallName, e.Token)
}

// ErrNoMatchingSlice indicates that a Fat binary does not contain a slice
// matching the native architecture.
type ErrNoMatchingSlice struct {
	BinaryPath string
	GOARCH     string
}

func (e *ErrNoMatchingSlice) Error() string {
	return fmt.Sprintf("no matching architecture slice in Fat binary: %s (GOARCH=%s)",
		e.BinaryPath, e.GOARCH)
}
