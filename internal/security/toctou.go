package security

import (
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
)

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

// RunTOCTOUPermissionCheck checks all collected directories for TOCTOU-exploitable
// permission issues. Non-existent directories are silently skipped (they cannot be
// exploited). Violations are logged as warnings and returned as a slice.
func RunTOCTOUPermissionCheck(checker DirectoryPermChecker, dirs []string, logger *slog.Logger) []TOCTOUViolation {
	if logger == nil {
		panic("RunTOCTOUPermissionCheck: logger must not be nil")
	}

	var violations []TOCTOUViolation

	for _, dir := range dirs {
		if err := checker.ValidateDirectoryPermissions(dir); err != nil {
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
