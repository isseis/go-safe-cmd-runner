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

// CollectTOCTOUCheckDirs collects directories to check for TOCTOU prevention.
// It returns deduplicated list of directories derived from:
//   - Parent directories of each path in verifyFilePaths
//   - Parent directories of each path in commandPaths
//   - hashDir itself
//   - All ancestor directories of hashDir up to root
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

	// Parent directories of verify_files entries
	for _, p := range verifyFilePaths {
		add(filepath.Dir(p))
	}

	// Parent directories of command paths
	for _, p := range commandPaths {
		add(filepath.Dir(p))
	}

	// hashDir itself and all ancestor directories up to root
	if hashDir != "" {
		cur := filepath.Clean(hashDir)
		for {
			add(cur)
			parent := filepath.Dir(cur)
			if parent == cur {
				// Reached filesystem root
				break
			}
			cur = parent
		}
	}

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
