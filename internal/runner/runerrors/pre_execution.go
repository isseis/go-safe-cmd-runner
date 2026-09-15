package runerrors

import (
	"errors"
	"fmt"
	"slices"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// NewVerificationPreExecutionError converts a verification failure into the
// pre-execution error shape shared by the global and group report boundaries.
// It is the only construction point, so the summary template, the failed-target
// copy, and the Component cannot drift between the two boundaries.
//
// The message carries the verification counts and the sentinel cause but no
// file paths; the paths travel in FailedFilePaths. Collection failures report
// the collection stage counts (unresolved targets and total targets) instead
// of a verification summary, because no file is verified on that path. Err is
// left nil so Detail returns Message unchanged.
func NewVerificationPreExecutionError(
	verErr *verification.Error,
	errType logging.ErrorType,
	scope common.NotificationContext,
	runID string,
) *logging.PreExecutionError {
	message := fmt.Sprintf("Total: %d, Verified: %d, Failed: %d, Error: %v",
		verErr.TotalFiles, verErr.VerifiedFiles, verErr.FailedFiles, verErr.Err)
	if errors.Is(verErr.Err, verification.ErrGroupVerificationCollectionFailed) {
		message = fmt.Sprintf("Collection failed: %d of %d targets unresolved, Error: %v",
			verErr.FailedFiles, verErr.TotalFiles, verErr.Err)
	}

	return &logging.PreExecutionError{
		Type:                errType,
		Message:             message,
		Component:           string(resource.ComponentVerification),
		RunID:               runID,
		NotificationContext: scope,
		FailedFilePaths:     slices.Clone(verErr.Details),
	}
}
