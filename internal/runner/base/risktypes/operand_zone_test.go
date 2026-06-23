package risktypes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPathTrustZone_StringValues pins the string value of each trust zone. The
// values are part of the audit record and must stay stable across changes.
func TestPathTrustZone_StringValues(t *testing.T) {
	assert.Equal(t, "trust-critical", string(ZoneTrustCritical))
	assert.Equal(t, "ordinary", string(ZoneOrdinary))
	assert.Equal(t, "safe-zone", string(ZoneSafeZone))
	assert.Equal(t, "unresolved", string(ZoneUnresolved))
}

// TestOperandRole_StringValues pins the string value of each operand role.
func TestOperandRole_StringValues(t *testing.T) {
	assert.Equal(t, "write", string(OperandRoleWrite))
	assert.Equal(t, "read", string(OperandRoleRead))
}

// TestOperandZone_ZeroValue documents the zero value: an empty record carries no
// resolution, an empty (zero) zone/role, and is not Trusted. This is the shape a
// caller sees before any classification is applied.
func TestOperandZone_ZeroValue(t *testing.T) {
	var oz OperandZone

	assert.Equal(t, 0, oz.Index)
	assert.Empty(t, oz.Raw)
	assert.Empty(t, oz.Resolved)
	assert.Equal(t, PathTrustZone(""), oz.Zone)
	assert.Equal(t, OperandRole(""), oz.Role)
	assert.Empty(t, oz.MatchedCritical)
	assert.False(t, oz.Trusted)
	assert.Empty(t, oz.UnresolvedErr)
}

// TestRunAsIdent_ZeroValue documents the zero value of the injected identity. The
// zoning logic must not treat the zero value as "unset" (see AC-21 / design 3.6);
// this test only pins the struct's zero shape.
func TestRunAsIdent_ZeroValue(t *testing.T) {
	var id RunAsIdent

	assert.Equal(t, uint32(0), id.UID)
	assert.Equal(t, uint32(0), id.GID)
	assert.Nil(t, id.Groups)
}

// TestRiskAssessment_OperandZonesEmptyByDefault documents the empty-vs-applied
// contract: a fresh RiskAssessment has no operand zones, meaning axis 2 did not
// apply (see OperandZone doc comment, consumed by task 0143).
func TestRiskAssessment_OperandZonesEmptyByDefault(t *testing.T) {
	var ra RiskAssessment
	assert.Empty(t, ra.OperandZones)
}
