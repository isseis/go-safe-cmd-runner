package risktypes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestReasonCodes_AllDistinct verifies that every defined reason code has a
// distinct, non-empty string value. A duplicate value would make two different
// reasons indistinguishable in the audit log.
func TestReasonCodes_AllDistinct(t *testing.T) {
	all := []ReasonCode{
		ReasonDestructiveFileOperation,
		ReasonSystemModification,
		ReasonPrivilegeEscalation,
		ReasonCoreutilsClassification,
		ReasonProfilePrivilege,
		ReasonProfileDestruction,
		ReasonProfileDataExfil,
		ReasonProfileNetwork,
		ReasonProfileSystemMod,
		ReasonBinaryAnalysisNetwork,
		ReasonBinaryAnalysisDynamicLoad,
		ReasonBinaryAnalysisExec,
		ReasonBinaryAnalysisSVC,
		ReasonBinaryAnalysisMprotectExec,
		ReasonUncertainMissingRecord,
		ReasonUncertainSchemaMismatch,
		ReasonUncertainHashMismatch,
		ReasonUncertainUnsupportedFormat,
		ReasonUncertainUnverifiedIdentity,
		ReasonAnalysisDisabled,
		ReasonArbitraryCodeExecution,
		ReasonDangerousArgPattern,
		ReasonSymlinkResolutionFailed,
		ReasonIdentityUnbound,
		ReasonIndirectExecutionRejected,
		ReasonIndirectExecutionWrapper,
		ReasonForbiddenEnvVar,
		ReasonNonAbsolutePath,
		ReasonNetworkArgument,
		ReasonTrustBoundaryWrite,
		ReasonDestinationZone,
		ReasonPermissionGrant,
		ReasonDeviceIO,
		ReasonRecursiveOutsideSafeZone,
		ReasonSensitiveSourceCopy,
		ReasonUnresolvedDestination,
	}

	seen := make(map[ReasonCode]struct{}, len(all))
	for _, rc := range all {
		assert.NotEmpty(t, string(rc), "reason code must have a non-empty string value")
		_, dup := seen[rc]
		assert.Falsef(t, dup, "duplicate reason code value: %q", rc)
		seen[rc] = struct{}{}
	}
}
