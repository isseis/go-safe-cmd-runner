package runerrors

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewVerificationPreExecutionError pins the single conversion from a
// verification failure to the shared pre-execution error shape: the two message
// templates, the fixed component, the copied failed targets, and the nil Err.
func TestNewVerificationPreExecutionError(t *testing.T) {
	tests := []struct {
		name        string
		verErr      *verification.Error
		errType     logging.ErrorType
		scope       common.NotificationContext
		runID       string
		wantMessage string
		wantPaths   []string
	}{
		{
			name: "group verification failure",
			verErr: &verification.Error{
				Op:            "group",
				Group:         "backup",
				Details:       []string{"/a", "/b"},
				TotalFiles:    3,
				VerifiedFiles: 2,
				FailedFiles:   1,
				Err:           verification.ErrGroupVerificationFailed,
			},
			errType:     logging.ErrorTypeGroupFileVerification,
			scope:       common.GroupScope("backup"),
			runID:       "run-1",
			wantMessage: "Total: 3, Verified: 2, Failed: 1, Error: group file verification failed",
			wantPaths:   []string{"/a", "/b"},
		},
		{
			name: "global verification failure uses the same template",
			verErr: &verification.Error{
				Op:            "global",
				Details:       []string{"/x"},
				TotalFiles:    1,
				VerifiedFiles: 0,
				FailedFiles:   1,
				Err:           verification.ErrGlobalVerificationFailed,
			},
			errType:     logging.ErrorTypeFileAccess,
			scope:       common.GlobalScope(),
			runID:       "run-2",
			wantMessage: "Total: 1, Verified: 0, Failed: 1, Error: global file verification failed",
			wantPaths:   []string{"/x"},
		},
		{
			name: "collection failure reports the collection stage counts",
			verErr: &verification.Error{
				Op:            "group",
				Group:         "backup",
				Details:       []string{"/nonexistent"},
				TotalFiles:    3,
				VerifiedFiles: 0,
				FailedFiles:   1,
				Err:           verification.ErrGroupVerificationCollectionFailed,
			},
			errType:     logging.ErrorTypeGroupFileVerification,
			scope:       common.GroupScope("backup"),
			runID:       "run-3",
			wantMessage: "Collection failed: 1 of 3 targets unresolved, Error: failed to collect verification files",
			wantPaths:   []string{"/nonexistent"},
		},
		{
			name: "empty details keep the summary template",
			verErr: &verification.Error{
				Op:  "global",
				Err: verification.ErrGlobalVerificationFailed,
			},
			errType:     logging.ErrorTypeFileAccess,
			scope:       common.GlobalScope(),
			runID:       "run-4",
			wantMessage: "Total: 0, Verified: 0, Failed: 0, Error: global file verification failed",
			wantPaths:   nil,
		},
		{
			name: "empty details keep the collection template",
			verErr: &verification.Error{
				Op:  "group",
				Err: verification.ErrGroupVerificationCollectionFailed,
			},
			errType:     logging.ErrorTypeGroupFileVerification,
			scope:       common.GroupScope("backup"),
			runID:       "run-5",
			wantMessage: "Collection failed: 0 of 0 targets unresolved, Error: failed to collect verification files",
			wantPaths:   nil,
		},
		{
			name: "preserves the given order without sorting",
			verErr: &verification.Error{
				Op:          "group",
				Details:     []string{"/c", "/a", "/b"},
				TotalFiles:  3,
				FailedFiles: 3,
				Err:         verification.ErrGroupVerificationFailed,
			},
			errType:     logging.ErrorTypeGroupFileVerification,
			scope:       common.GroupScope("backup"),
			runID:       "run-6",
			wantMessage: "Total: 3, Verified: 0, Failed: 3, Error: group file verification failed",
			wantPaths:   []string{"/c", "/a", "/b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewVerificationPreExecutionError(tt.verErr, tt.errType, tt.scope, tt.runID)

			require.NotNil(t, got)
			assert.Equal(t, tt.errType, got.Type)
			assert.Equal(t, tt.wantMessage, got.Message)
			assert.Equal(t, tt.wantPaths, got.FailedFilePaths)
			assert.Equal(t, string(resource.ComponentVerification), got.Component)
			assert.Equal(t, tt.scope, got.NotificationContext)
			assert.Equal(t, tt.runID, got.RunID)
			assert.Nil(t, got.Err, "the cause is embedded in Message, not wrapped")
		})
	}

	t.Run("copies the failed targets", func(t *testing.T) {
		verErr := &verification.Error{
			Op:          "group",
			Details:     []string{"/a"},
			TotalFiles:  2,
			FailedFiles: 1,
			Err:         verification.ErrGroupVerificationFailed,
		}

		got := NewVerificationPreExecutionError(verErr, logging.ErrorTypeGroupFileVerification, common.GroupScope("backup"), "run-5")
		verErr.Details[0] = "/mutated"

		assert.Equal(t, []string{"/a"}, got.FailedFilePaths,
			"the reported list must not alias the verification error's details")
	})
}
