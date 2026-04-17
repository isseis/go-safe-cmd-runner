package elfdynlib

import (
	"fmt"
	"strings"
)

// ErrDTRPATHNotSupported indicates that an ELF binary or shared library uses
// DT_RPATH, which is not supported. Use DT_RUNPATH instead (link with
// -Wl,--enable-new-dtags or omit -Wl,--disable-new-dtags).
type ErrDTRPATHNotSupported struct {
	Path  string // path of the ELF file that contains DT_RPATH
	RPATH string // the DT_RPATH value
}

func (e *ErrDTRPATHNotSupported) Error() string {
	return fmt.Sprintf("DT_RPATH is not supported: %s has DT_RPATH=%q\n"+
		"  relink with -Wl,--enable-new-dtags to use DT_RUNPATH instead",
		e.Path, e.RPATH)
}

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
