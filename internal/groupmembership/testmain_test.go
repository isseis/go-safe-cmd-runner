//go:build test

package groupmembership

import (
	"os"
	"testing"
)

// TestMain settles the completeness classification once before any test runs,
// the way every binary does at startup through EnsurePermissionCheckUID or
// PrecomputeEnumerationEnvironment. Without it the tests that enumerate for
// real would read the unstated zero value and deny, which is production's
// answer for a binary that never settled the classification -- not for one
// that did.
func TestMain(m *testing.M) {
	settleStartupNsswitchClassification()
	os.Exit(m.Run())
}
