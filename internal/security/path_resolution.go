package security

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// trimLastPathComponent drops the final component of p without cleaning it, and
// reports whether there was anything left to drop.
//
// filepath.Dir is unusable here because it cleans: it would collapse ".."
// textually, which is exactly what walking the raw path avoids.
func trimLastPathComponent(p string) (string, bool) {
	root := string(filepath.Separator)
	i := strings.LastIndexByte(p, filepath.Separator)
	switch {
	case i < 0 || p == root:
		return "", false
	case i == 0:
		return root, true
	default:
		return p[:i], true
	}
}

// DeepestExistingAncestor returns the deepest existing entry at or above path,
// which must be absolute. The result is a literal prefix of path and is not
// cleaned: the walk trims one component at a time and lets os.Lstat resolve each
// candidate, so a ".." that follows a symlink is resolved the way the kernel
// resolves it rather than collapsed textually.
//
// Existence is tested with os.Lstat, so a dangling symlink counts as existing:
// reporting it as unresolvable is preferable to silently checking the directory
// tree the link sits in rather than the one it points at.
//
// A component that cannot be stat'ed for a reason other than "does not exist"
// (an unsearchable ancestor, say) is returned as an error; the walk never
// reaches past it.
func DeepestExistingAncestor(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %s is not absolute", ErrInvalidPath, path)
	}
	cur := path
	for {
		_, err := os.Lstat(cur)
		if err == nil {
			return cur, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent, ok := trimLastPathComponent(cur)
		if !ok {
			// The root itself is missing; there is nothing left to walk up to.
			return "", err
		}
		cur = parent
	}
}

// hasDotDotComponent reports whether p has ".." as one of its components.
func hasDotDotComponent(p string) bool {
	for component := range strings.SplitSeq(p, string(filepath.Separator)) {
		if component == ".." {
			return true
		}
	}
	return false
}

// ResolvePathForCheck returns the path to hand to the directory permission
// check. A relative path is made absolute against the process working
// directory. When the target does not exist yet, symlinks are resolved as far as
// the deepest existing ancestor and the remainder is appended, so the
// directories that will actually contain the target are the ones checked.
//
// The path is deliberately never cleaned, since cleaning collapses ".."
// textually and would send the check to the tree a symlink sits in rather than
// the one it points at. For the same reason a ".." inside the not-yet-existing
// remainder is rejected rather than collapsed: it can pop back above the missing
// components onto a symlink that no walk has resolved.
//
// An empty path names nothing to check and is rejected with ErrInvalidPath;
// callers that use "" to mean "skip" must not reach here. Every other failure
// returns a checkable path together with an error wrapping ErrPathResolution.
// Dropping the path instead would let a path that is a fail-closed premise go
// silently unchecked.
func ResolvePathForCheck(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%w: empty path has nothing to check", ErrInvalidPath)
	}

	abs := path
	if !filepath.IsAbs(path) {
		wd, err := getwd()
		if err != nil {
			return path, fmt.Errorf("%w: %s: %w", ErrPathResolution, path, err)
		}
		// Concatenated rather than joined: filepath.Join cleans the whole result,
		// which would collapse a ".." inside path itself and reintroduce the
		// confusion the raw walk below exists to avoid.
		root := string(filepath.Separator)
		abs = strings.TrimSuffix(wd, root) + root + path
	}

	ancestor, err := DeepestExistingAncestor(abs)
	if err != nil {
		return abs, fmt.Errorf("%w: %s: %w", ErrPathResolution, path, err)
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return abs, fmt.Errorf("%w: %s: %w", ErrPathResolution, path, err)
	}
	// The ancestor is a literal prefix of abs, so the remainder is the tail after
	// it. None of its components exist, so none of them can be a symlink — unless
	// a ".." leaves the missing part of the path altogether, which no lexical join
	// can resolve. Reject that rather than repair it, and hand back the existing
	// ancestor, which is a tree the target really is under.
	rest := strings.TrimLeft(abs[len(ancestor):], string(filepath.Separator))
	if rest == "" {
		return resolvedAncestor, nil
	}
	if hasDotDotComponent(rest) {
		return resolvedAncestor, fmt.Errorf("%w: %s: %w: %q escapes the missing part of the path", ErrPathResolution, path, ErrInvalidPath, rest)
	}
	// Safe now that ".." is gone: joining can only drop "." and repeated separators.
	return filepath.Join(resolvedAncestor, rest), nil
}

// ResolveAllForCheck resolves a list of paths with ResolvePathForCheck and
// returns the number that failed. One WARN is logged per failure. The returned
// slice has the same length as the input; failed entries still carry a checkable
// path.
func ResolveAllForCheck(paths []string, logger *slog.Logger) (resolved []string, failures int) {
	if logger == nil {
		panic("ResolveAllForCheck: logger must not be nil")
	}

	resolved = make([]string, 0, len(paths))
	for _, p := range paths {
		r, err := ResolvePathForCheck(p)
		if err != nil {
			failures++
			logger.Warn("failed to resolve path for permission check",
				slog.String("path", p),
				slog.String("checked_path", r),
				slog.String("error", err.Error()),
			)
		}
		resolved = append(resolved, r)
	}
	return resolved, failures
}
