package dynlibanalysis

import (
	"path/filepath"
	"strings"
)

// ResolveContext holds the resolution context for a specific DT_NEEDED entry.
// It tracks the RPATH/RUNPATH of the parent ELF and the inherited RPATH chain
// from ancestor ELFs.
type ResolveContext struct {
	// ParentPath is the full path of the ELF whose DT_NEEDED is being resolved.
	ParentPath string

	// ParentDir is filepath.Dir(ParentPath), used for $ORIGIN expansion
	// of the parent's own RPATH/RUNPATH.
	ParentDir string

	// OwnRPATH is the DT_RPATH of ParentPath (empty if DT_RUNPATH is present).
	OwnRPATH []string

	// OwnRUNPATH is the DT_RUNPATH of ParentPath.
	OwnRUNPATH []string

	// InheritedRPATH is the ordered list of RPATH entries inherited from
	// ancestor ELFs. Each entry is an ExpandedRPATHEntry containing the
	// search path and the originating ELF path (for $ORIGIN expansion).
	// Inheritance is terminated when the loading object itself has DT_RUNPATH
	// (see NewChildContext). For the full inheritance rules, refer to:
	// docs/dev/elf-rpath-runpath-inheritance.md
	InheritedRPATH []ExpandedRPATHEntry

	// IncludeLDLibraryPath controls whether LD_LIBRARY_PATH is consulted.
	// false at record time, true at runner time.
	IncludeLDLibraryPath bool
}

// ExpandedRPATHEntry is an RPATH entry with its originating ELF path,
// needed for correct $ORIGIN expansion of inherited RPATH entries.
type ExpandedRPATHEntry struct {
	// Path is the RPATH entry (may contain $ORIGIN).
	Path string
	// OriginDir is the directory of the ELF that owns this RPATH entry.
	OriginDir string
}

// NewRootContext creates a ResolveContext for resolving the DT_NEEDED entries
// of the root binary (the command being analyzed).
func NewRootContext(binaryPath string, rpath, runpath []string, includeLDLibraryPath bool) *ResolveContext {
	ctx := &ResolveContext{
		ParentPath:           binaryPath,
		ParentDir:            filepath.Dir(binaryPath),
		IncludeLDLibraryPath: includeLDLibraryPath,
	}
	// DT_RUNPATH takes priority: when present, DT_RPATH is ignored
	if len(runpath) > 0 {
		ctx.OwnRUNPATH = runpath
	} else {
		ctx.OwnRPATH = rpath
	}
	return ctx
}

// NewChildContext creates a ResolveContext for resolving the DT_NEEDED entries
// of a resolved library (i.e., for finding the grandchildren of c.ParentPath).
// It computes the RPATH inheritance chain according to ld.so rules:
//
//   - DT_RPATH of an ELF is propagated to the resolution of all transitive
//     dependencies, UNLESS the loading object itself has DT_RUNPATH.
//   - DT_RUNPATH is NOT inherited: it applies only to the direct DT_NEEDED
//     entries of the ELF that carries it, not to their children.
//   - When child has DT_RUNPATH, the RPATH inheritance chain is terminated
//     for child's own DT_NEEDED resolution: neither child's own DT_RPATH
//     (which is overridden by DT_RUNPATH) nor any ancestor's RPATH is used.
//     This matches glibc's behaviour: the loader's RPATH chain is consulted
//     only if the loading object (child) itself has no DT_RUNPATH.
func (c *ResolveContext) NewChildContext(
	childPath string,
	childRPATH []string,
	childRUNPATH []string,
) *ResolveContext {
	child := &ResolveContext{
		ParentPath:           childPath,
		ParentDir:            filepath.Dir(childPath),
		IncludeLDLibraryPath: c.IncludeLDLibraryPath,
	}

	// Set the child's own RPATH/RUNPATH
	if len(childRUNPATH) > 0 {
		child.OwnRUNPATH = childRUNPATH
		// When child has DT_RUNPATH, glibc does NOT walk up the loader RPATH
		// chain for child's DT_NEEDED resolution. Therefore InheritedRPATH
		// must be left nil — carrying the ancestor chain forward would cause
		// false positives when the record-time and run-time loader environments
		// differ (e.g. LD_LIBRARY_PATH hijack detection).
	} else {
		child.OwnRPATH = childRPATH

		// Build inherited RPATH chain: start with parent's inherited, then add parent's own RPATH.
		// (Parent's own RPATH is only contributed when parent itself has no RUNPATH,
		// which is guaranteed here because c.OwnRUNPATH is set only when RUNPATH was
		// present, making c.OwnRPATH empty in that case.)
		//
		// Duplicate expanded paths are skipped: the same expanded path can only
		// appear once in the chain (duplicates arise in circular dependency graphs
		// where the same RPATH-bearing ELF is revisited, and re-adding it would
		// produce an ever-growing chain that never converges to a stable fingerprint).
		inherited := make([]ExpandedRPATHEntry, 0, len(c.InheritedRPATH)+len(c.OwnRPATH))
		seen := make(map[string]struct{}, len(c.InheritedRPATH)+len(c.OwnRPATH))
		for _, entry := range c.InheritedRPATH {
			expanded := expandOrigin(entry.Path, entry.OriginDir)
			if _, dup := seen[expanded]; !dup {
				seen[expanded] = struct{}{}
				inherited = append(inherited, entry)
			}
		}

		// Parent's own RPATH (if it had no RUNPATH) is inherited by child
		if len(c.OwnRUNPATH) == 0 {
			for _, rp := range c.OwnRPATH {
				expanded := expandOrigin(rp, c.ParentDir)
				if _, dup := seen[expanded]; !dup {
					seen[expanded] = struct{}{}
					inherited = append(inherited, ExpandedRPATHEntry{
						Path:      rp,
						OriginDir: c.ParentDir,
					})
				}
			}
		}
		// If parent had RUNPATH (c.OwnRUNPATH is set), parent's OwnRPATH is empty
		// and OwnRUNPATH is not inherited, so we only carry forward existing inherited.
		child.InheritedRPATH = inherited
	}

	return child
}

// rpathFingerprint returns a string that uniquely identifies the RPATH search
// paths this context will use when resolving its DT_NEEDED entries. Two
// contexts with identical fingerprints will resolve any given soname to the
// same path, so their child traversals can be deduplicated.
//
// The fingerprint is the ":"-joined list of expanded search paths in resolution
// order: OwnRPATH (when RUNPATH absent), InheritedRPATH, then OwnRUNPATH.
// LD_LIBRARY_PATH is intentionally excluded because it is always false at
// record time (IncludeLDLibraryPath=false).
func (c *ResolveContext) rpathFingerprint() string {
	var parts []string
	if len(c.OwnRUNPATH) == 0 {
		for _, rp := range c.OwnRPATH {
			parts = append(parts, expandOrigin(rp, c.ParentDir))
		}
		for _, entry := range c.InheritedRPATH {
			parts = append(parts, expandOrigin(entry.Path, entry.OriginDir))
		}
	} else {
		for _, rp := range c.OwnRUNPATH {
			parts = append(parts, expandOrigin(rp, c.ParentDir))
		}
	}
	return strings.Join(parts, ":")
}

// expandOrigin replaces $ORIGIN and ${ORIGIN} in the given path with the
// specified directory. glibc accepts both syntaxes (see ld.so(8)).
func expandOrigin(path string, originDir string) string {
	result := strings.ReplaceAll(path, "${ORIGIN}", originDir)
	return strings.ReplaceAll(result, "$ORIGIN", originDir)
}
