package security

import (
	"fmt"
	"path/filepath"
)

// CheckSkipReason states why a configured path was left out of the directory
// permission check. The zero value means it was not left out, so a value added
// later defaults to being checked.
type CheckSkipReason int

const (
	// CheckSkipNone means the path can be checked.
	CheckSkipNone CheckSkipReason = iota
	// CheckSkipVariableReference means the path still holds an unexpanded
	// variable reference, so its real location is not yet known.
	CheckSkipVariableReference
	// CheckSkipRelative means the path is relative. Configured relative paths
	// are not anchored to the process working directory, so making them
	// absolute against it would check an unrelated directory tree.
	CheckSkipRelative
)

// PathExpansionState declares what the caller knows about variable references in
// a configured path. This package cannot work it out from the path itself: the
// configuration syntax has an escape ("\%{" expands to a literal "%{"), so the
// presence of "%{" is not evidence of an unexpanded reference, and the caller's
// own position in the pipeline — before or after expansion — is not visible
// here either. Only the configuration layer knows, so it says so.
//
// The zero value declares the least it can about its input: that the path is
// whatever it appears to be, and is therefore checked.
type PathExpansionState int

const (
	// PathExpanded declares that the path holds no unexpanded variable
	// reference, so its final location is the one written.
	PathExpanded PathExpansionState = iota
	// PathHasUnexpandedReference declares that the path still holds a variable
	// reference, so where it will finally point is not yet known.
	PathHasUnexpandedReference
)

// ClassifyCheckTarget reports whether a configured path can be used as a
// permission check target, and if not, why.
func ClassifyCheckTarget(path string, state PathExpansionState) CheckSkipReason {
	switch state {
	case PathHasUnexpandedReference:
		// Reported before the absolute test: an unexpanded reference is unusable
		// whether or not the surrounding text happens to start with a separator.
		return CheckSkipVariableReference
	case PathExpanded:
		// Fall through to the tests on the path itself.
	default:
		// A state added without a case here would otherwise land silently in one
		// of the branches above; refusing to run is the only safe reading.
		panic(fmt.Sprintf("ClassifyCheckTarget: unhandled PathExpansionState %d", state))
	}

	if !filepath.IsAbs(path) {
		return CheckSkipRelative
	}
	return CheckSkipNone
}

// CollectPermissionCheckDirs collects the directories to hand to a directory
// permission check. Each entry of filePaths contributes its parent directory,
// each entry of dirs contributes itself, and every ancestor up to the root is
// added as well, because an attacker with write access to any ancestor can
// rename an intermediate directory to bypass protection on the immediate parent.
// The result is cleaned and deduplicated, in first-seen order. Empty strings are
// ignored, which is how callers say "nothing to add here".
//
// Nothing is stat'ed and no permission is inspected: this is path arithmetic
// only.
func CollectPermissionCheckDirs(filePaths []string, dirs []string) []string {
	seen := make(map[string]struct{})
	var result []string

	add := func(dir string) {
		if dir == "" {
			return
		}
		clean := filepath.Clean(dir)
		if _, exists := seen[clean]; !exists {
			seen[clean] = struct{}{}
			result = append(result, clean)
		}
	}

	addWithAncestors := func(dir string) {
		if dir == "" {
			return
		}
		cur := filepath.Clean(dir)
		for {
			if _, exists := seen[cur]; exists {
				break
			}
			add(cur)
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}

	for _, p := range filePaths {
		if p == "" {
			continue
		}
		addWithAncestors(filepath.Dir(p))
	}

	for _, d := range dirs {
		addWithAncestors(d)
	}

	return result
}
