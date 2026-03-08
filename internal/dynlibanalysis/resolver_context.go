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

	// OwnRUNPATH is the DT_RUNPATH of ParentPath.
	OwnRUNPATH []string
}

// NewResolveContext creates a ResolveContext for resolving DT_NEEDED entries
// of the given ELF path.
func NewResolveContext(parentPath string, runpath []string) *ResolveContext {
	return &ResolveContext{
		ParentPath: parentPath,
		OwnRUNPATH: runpath,
	}
}

// ParentDir returns filepath.Dir(ParentPath) for $ORIGIN expansion.
func (c *ResolveContext) ParentDir() string {
	return filepath.Dir(c.ParentPath)
}

// expandOrigin replaces $ORIGIN and ${ORIGIN} in the given path with the
// specified directory. glibc accepts both syntaxes (see ld.so(8)).
func expandOrigin(path string, originDir string) string {
	result := strings.ReplaceAll(path, "${ORIGIN}", originDir)
	return strings.ReplaceAll(result, "$ORIGIN", originDir)
}
