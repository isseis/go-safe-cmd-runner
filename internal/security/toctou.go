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

// variableReferencePrefix opens an unexpanded configuration variable reference.
const variableReferencePrefix = "%{"

// TOCTOUViolation contains information about a TOCTOU permission check violation.
type TOCTOUViolation struct {
	Path string
	Err  error
}

// ResolveAbsPathForTOCTOU normalizes an already-absolute path for TOCTOU
// directory collection.
func ResolveAbsPathForTOCTOU(p string) (string, bool) {
	if !filepath.IsAbs(p) {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, true
	}
	return p, true
}

// DeepestExistingAncestor returns the deepest existing entry at or above path,
// which must be absolute. Existence is tested with os.Lstat, so a dangling
// symlink counts as existing: reporting it as unresolvable is preferable to
// silently checking the directory tree the link sits in rather than the one it
// points at.
//
// A component that cannot be stat'ed for a reason other than "does not exist"
// (an unsearchable ancestor, say) is returned as an error; the walk never
// reaches past it.
func DeepestExistingAncestor(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %s is not absolute", ErrInvalidPath, path)
	}
	cur := filepath.Clean(path)
	for {
		_, err := os.Lstat(cur)
		if err == nil {
			return cur, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// The root itself is missing; there is nothing left to walk up to.
			return "", err
		}
		cur = parent
	}
}

// ResolvePathForCheck returns the path to hand to the directory permission
// check. A relative path is made absolute against the process working
// directory. When the target does not exist yet, symlinks are resolved as far as
// the deepest existing ancestor and the remainder is joined lexically, so the
// directories that will actually contain the target are the ones checked.
//
// A checkable path is returned even on failure, together with an error wrapping
// ErrPathResolution. Dropping the path instead would let a path that is a
// fail-closed premise go silently unchecked.
func ResolvePathForCheck(path string) (string, error) {
	abs := path
	if !filepath.IsAbs(path) {
		wd, err := getwd()
		if err != nil {
			return path, fmt.Errorf("%w: %s: %w", ErrPathResolution, path, err)
		}
		abs = filepath.Join(wd, path)
	}
	abs = filepath.Clean(abs)

	ancestor, err := DeepestExistingAncestor(abs)
	if err != nil {
		return abs, fmt.Errorf("%w: %s: %w", ErrPathResolution, path, err)
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return abs, fmt.Errorf("%w: %s: %w", ErrPathResolution, path, err)
	}
	rest, err := filepath.Rel(ancestor, abs)
	if err != nil {
		return abs, fmt.Errorf("%w: %s: %w", ErrPathResolution, path, err)
	}
	if rest == "." {
		return resolvedAncestor, nil
	}
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

// ClassifyCheckTarget reports whether a configured path can be used as a
// permission check target, and if not, why.
func ClassifyCheckTarget(path string) CheckSkipReason {
	// Checked before the absolute test: an unexpanded reference is unusable
	// whether or not the surrounding text happens to start with a separator.
	if strings.Contains(path, variableReferencePrefix) {
		return CheckSkipVariableReference
	}
	if !filepath.IsAbs(path) {
		return CheckSkipRelative
	}
	return CheckSkipNone
}

// CollectTOCTOUCheckDirs collects directories to check for TOCTOU prevention.
// Returns a deduplicated list of directories covering the parent and all ancestor
// directories up to root for each entry, because an attacker with write access to
// any ancestor can rename an intermediate directory to bypass protection on the
// immediate parent.
func CollectTOCTOUCheckDirs(verifyFilePaths []string, commandPaths []string, hashDir string) []string {
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

	for _, p := range verifyFilePaths {
		addWithAncestors(filepath.Dir(p))
	}

	for _, p := range commandPaths {
		addWithAncestors(filepath.Dir(p))
	}

	addWithAncestors(hashDir)

	return result
}

// TOCTOUCheckResult reports the outcome of a directory permission check run.
// Checked and Skipped let a caller state how much of what it collected was
// actually inspected, which "no violations" on its own does not say.
type TOCTOUCheckResult struct {
	Violations []TOCTOUViolation
	Checked    int // directories that existed and were inspected
	Skipped    int // directories that did not exist and were skipped
}

// RunTOCTOUPermissionCheck checks all collected directories for TOCTOU-exploitable
// permission issues. Non-existent directories are silently skipped (they cannot be
// exploited) and counted in Skipped. Violations are logged as warnings; a directory
// that violates the policy was still inspected, so it counts towards Checked too.
func RunTOCTOUPermissionCheck(checker DirectoryPermChecker, dirs []string, logger *slog.Logger) TOCTOUCheckResult {
	if logger == nil {
		panic("RunTOCTOUPermissionCheck: logger must not be nil")
	}

	var result TOCTOUCheckResult

	for _, dir := range dirs {
		err := checker.ValidateDirectoryPermissions(dir)
		if err == nil {
			result.Checked++
			continue
		}
		if errors.Is(err, fs.ErrNotExist) {
			result.Skipped++
			continue
		}
		result.Checked++
		logger.Warn("TOCTOU permission check violation",
			slog.String("path", dir),
			slog.String("violation", err.Error()),
		)
		result.Violations = append(result.Violations, TOCTOUViolation{Path: dir, Err: err})
	}

	return result
}
