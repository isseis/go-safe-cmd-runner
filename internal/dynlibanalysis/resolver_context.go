package dynlibanalysis

import (
	"path/filepath"
	"strings"
)

// ResolveContext holds the resolution context for a specific DT_NEEDED entry.
// It tracks the RUNPATH of the parent ELF.
//
// DT_RPATH is not supported; ELF files containing DT_RPATH cause an error
// at Analyze time (see ErrDTRPATHNotSupported). Only DT_RUNPATH is used.
//
// LD_LIBRARY_PATH is not used: record time ignores it, and runner clears it
// before executing commands. No LD_LIBRARY_PATH support is needed.
type ResolveContext struct {
	// ParentPath is the full path of the ELF whose DT_NEEDED is being resolved.
	ParentPath string

	// ParentDir is filepath.Dir(ParentPath), used for $ORIGIN expansion.
	ParentDir string

	// OwnRUNPATH is the DT_RUNPATH of ParentPath.
	OwnRUNPATH []string
}

// NewRootContext creates a ResolveContext for resolving the DT_NEEDED entries
// of the root binary (the command being analyzed).
func NewRootContext(binaryPath string, runpath []string) *ResolveContext {
	return &ResolveContext{
		ParentPath: binaryPath,
		ParentDir:  filepath.Dir(binaryPath),
		OwnRUNPATH: runpath,
	}
}

// NewChildContext creates a ResolveContext for resolving the DT_NEEDED entries
// of a resolved library.
func (c *ResolveContext) NewChildContext(childPath string, childRUNPATH []string) *ResolveContext {
	return &ResolveContext{
		ParentPath: childPath,
		ParentDir:  filepath.Dir(childPath),
		OwnRUNPATH: childRUNPATH,
	}
}

// expandOrigin replaces $ORIGIN and ${ORIGIN} in the given path with the
// specified directory. glibc accepts both syntaxes (see ld.so(8)).
func expandOrigin(path string, originDir string) string {
	result := strings.ReplaceAll(path, "${ORIGIN}", originDir)
	return strings.ReplaceAll(result, "$ORIGIN", originDir)
}
