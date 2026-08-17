package security

import (
	"errors"
	"io/fs"
	"log/slog"
)

// TOCTOUViolation contains information about a TOCTOU permission check violation.
type TOCTOUViolation struct {
	Path string
	Err  error
}

// TOCTOUCheckResult reports the outcome of a directory permission check run.
// Checked and Skipped let a caller state how much of what it collected was
// actually inspected, which "no violations" on its own does not say.
type TOCTOUCheckResult struct {
	Violations []TOCTOUViolation
	// Checked counts the directories the checker reached a verdict on, whether it
	// passed or was a violation. A directory it could not stat for a reason other
	// than absence counts here too: that is a verdict of "not trustworthy", not a
	// directory left uninspected.
	Checked int
	Skipped int // directories that did not exist and were skipped
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
		result.Violations = append(result.Violations, TOCTOUViolation{Path: dir, Err: err})
	}

	return result
}
