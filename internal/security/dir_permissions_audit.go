package security

import (
	"errors"
	"io/fs"
	"log/slog"
)

// DirPermViolation contains information about a directory permission violation.
type DirPermViolation struct {
	Path string
	Err  error
}

// DirPermAuditResult reports the outcome of a directory permission audit run.
// Checked and Skipped let a caller state how much of what it collected was
// actually inspected, which "no violations" on its own does not say.
type DirPermAuditResult struct {
	Violations []DirPermViolation
	// Checked counts the directories the checker reached a verdict on, whether it
	// passed or was a violation. A directory it could not stat for a reason other
	// than absence counts here too: that is a verdict of "not trustworthy", not a
	// directory left uninspected.
	Checked int
	Skipped int // directories that did not exist and were skipped
}

// AuditDirectoryPermissions inspects all collected directories for insecure
// permissions, ownership, and path components. Non-existent directories are
// silently skipped (they cannot be exploited) and counted in Skipped. Violations
// are logged as warnings; a directory that violates the policy was still
// inspected, so it counts towards Checked too.
//
// This is a static, one-shot audit performed ahead of use: each directory is
// examined once and a verdict is recorded. It is not a time-of-check/time-of-use
// comparison, and it establishes nothing about the directory at the moment a file
// under it is later opened or executed. Defence against that race lives elsewhere:
// safefileio opens with O_NOFOLLOW, and runner/base/executor executes through a
// file descriptor rather than re-resolving the path.
func AuditDirectoryPermissions(checker DirectoryPermChecker, dirs []string, logger *slog.Logger) DirPermAuditResult {
	if logger == nil {
		panic("AuditDirectoryPermissions: logger must not be nil")
	}

	var result DirPermAuditResult

	for _, dir := range dirs {
		err := checker.ValidateDirectoryPermissions(dir)
		if err == nil {
			result.Checked++
			continue
		}
		if errors.Is(err, fs.ErrNotExist) {
			// Counted, not reported: absence is not a verdict this function can
			// report as a fault, and Skipped is what tells a reader how much of the
			// collected set this run actually established. The checker records the
			// path itself, but at debug level, so nothing surfaces at the info level
			// the entry points run at -- reporting Skipped is the caller's job.
			result.Skipped++
			continue
		}
		result.Checked++
		logger.Warn("TOCTOU permission check violation",
			slog.String("path", dir),
			slog.String("violation", err.Error()),
		)
		result.Violations = append(result.Violations, DirPermViolation{Path: dir, Err: err})
	}

	return result
}
