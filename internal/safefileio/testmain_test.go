package safefileio

import (
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
)

// TestMain settles the group-enumeration completeness classification before
// any test runs, the way record, verify and runner do at startup. The
// write-safety decision denies while the classification is unstated, so a
// test binary that skipped this would be exercising the answer given to a
// binary that never settled it.
func TestMain(m *testing.M) {
	groupmembership.PrecomputeEnumerationEnvironment()
	os.Exit(m.Run())
}
