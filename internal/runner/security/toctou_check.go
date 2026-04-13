package security

import (
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
)

// NewValidatorForTOCTOU creates a Validator configured for TOCTOU directory
// permission checks.  It wires in real group membership support so that
// group-writable directories whose group has only one member are not
// incorrectly reported as violations.
func NewValidatorForTOCTOU() (*Validator, error) {
	return NewValidator(nil, WithGroupMembership(groupmembership.New()))
}

// TOCTOUViolation contains information about a TOCTOU permission check violation.
type TOCTOUViolation struct {
	Path string
	Err  error
}

// ResolveAbsPathForTOCTOU normalizes an already-absolute path for use in TOCTOU
// directory collection. Symlinks are resolved so that canonicalised paths can
// be compared without false positives (e.g. /bin -> /usr/bin). The second
// return value is false when p is not absolute and the path should be skipped.
func ResolveAbsPathForTOCTOU(p string) (string, bool) {
	if !filepath.IsAbs(p) {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, true
	}
	return p, true
}

// CollectTOCTOUCheckDirs collects directories to check for TOCTOU prevention.
// It returns deduplicated list of directories derived from:
//   - Parent directory and all ancestor directories up to root for each path in verifyFilePaths
//   - Parent directory and all ancestor directories up to root for each path in commandPaths
//   - hashDir itself and all ancestor directories up to root
//
// All three sources receive full ancestor traversal because an attacker with write
// access to any ancestor directory can rename an intermediate directory and bypass
// the protection on the immediate parent.
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

	// Traverse dir and all ancestor directories up to root.
	// Stops early when a directory already in seen is reached, since its
	// ancestors must also be present.
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
				// Reached filesystem root
				break
			}
			cur = parent
		}
	}

	// Parent directory and all ancestors of verify_files entries
	for _, p := range verifyFilePaths {
		addWithAncestors(filepath.Dir(p))
	}

	// Parent directory and all ancestors of command paths
	for _, p := range commandPaths {
		addWithAncestors(filepath.Dir(p))
	}

	// hashDir itself and all ancestor directories up to root
	addWithAncestors(hashDir)

	return result
}

// RunTOCTOUPermissionCheck checks all collected directories for TOCTOU-exploitable
// permission issues. Each directory is checked using ValidateDirectoryPermissions.
// Directories that do not exist are silently skipped (they cannot be exploited if absent).
// Violations are logged as warnings and returned as a slice.
func RunTOCTOUPermissionCheck(v *Validator, dirs []string, logger *slog.Logger) []TOCTOUViolation {
	var violations []TOCTOUViolation

	for _, dir := range dirs {
		if err := v.ValidateDirectoryPermissions(dir); err != nil {
			// Skip non-existent directories — they cannot be exploited if they don't exist yet.
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			logger.Warn("TOCTOU permission check violation",
				slog.String("path", dir),
				slog.String("violation", err.Error()),
			)
			violations = append(violations, TOCTOUViolation{Path: dir, Err: err})
		}
	}

	return violations
}
