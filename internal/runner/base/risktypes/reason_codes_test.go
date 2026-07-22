package risktypes

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// totalReasonCodes is the number of ReasonCode constants defined in
// reason_codes.go. It anchors the family table's size: Go cannot reflect over a
// const block, so a new ReasonCode left out of reasonFamilies would be invisible
// to every table-derived test. This count (kept in sync with the const block by
// hand) detects such an omission. Update it whenever a ReasonCode is added or
// removed.
const totalReasonCodes = 38

// allReasonCodes returns the canonical set of reason codes, derived from the
// family table's keys so there is no parallel hardcoded list to drift.
func allReasonCodes() []ReasonCode {
	return slices.Collect(maps.Keys(reasonFamilies))
}

// TestReasonCodes_AllDistinct verifies that every reason code has a non-empty
// string value and that no two codes share a value. Because the code set is
// derived from the family table's keys (which are ReasonCode values), code-level
// uniqueness is structural; the load-bearing distinctness check is that two
// constants assigned the same string literal would collapse to a single key,
// which the size anchor in TestReasonFamily_AllCodesAssigned catches. The
// value-uniqueness loop below also keeps teeth if the source set is ever changed
// back to an explicit slice.
func TestReasonCodes_AllDistinct(t *testing.T) {
	all := allReasonCodes()

	values := make(map[string]struct{}, len(all))
	for _, rc := range all {
		assert.NotEmpty(t, string(rc), "reason code must have a non-empty string value")
		_, dupVal := values[string(rc)]
		assert.Falsef(t, dupVal, "duplicate reason code string value: %q", rc)
		values[string(rc)] = struct{}{}
	}
}

// TestReasonFamily_AllCodesAssigned verifies every reason code in the table maps
// to one of the defined families and that the table has the expected size, so a
// newly added code that was not assigned a family is caught.
func TestReasonFamily_AllCodesAssigned(t *testing.T) {
	definedFamilies := map[ReasonFamily]struct{}{
		FamilyNameClassification: {},
		FamilyPrivilege:          {},
		FamilyBinaryAnalysis:     {},
		FamilyUncertain:          {},
		FamilyRuntimeArgument:    {},
		FamilyPathTrustZone:      {},
	}

	// Ground-truth anchor: the table must enumerate every ReasonCode constant.
	assert.Len(t, reasonFamilies, totalReasonCodes,
		"reasonFamilies must enumerate every ReasonCode (update totalReasonCodes when adding/removing a code)")

	for code, family := range reasonFamilies {
		assert.NotEmptyf(t, string(family), "reason code %q has an empty family", code)
		_, ok := definedFamilies[family]
		assert.Truef(t, ok, "reason code %q maps to an undefined family %q", code, family)
	}
}

// TestReasonFamily_OfReturnsAssignedFamily verifies FamilyOf returns the assigned
// family for every code in the table and reports an unknown code as unassigned.
func TestReasonFamily_OfReturnsAssignedFamily(t *testing.T) {
	for code, want := range reasonFamilies {
		got, ok := FamilyOf(code)
		assert.Truef(t, ok, "FamilyOf(%q) must report the code as assigned", code)
		assert.Equalf(t, want, got, "FamilyOf(%q) must return the assigned family", code)
	}

	got, ok := FamilyOf(ReasonCode("__nonexistent__"))
	assert.False(t, ok, "FamilyOf must report an unknown code as unassigned")
	assert.Empty(t, string(got), "FamilyOf must return an empty family for an unknown code")
}
