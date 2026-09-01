package main

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/sourceorder"
)

// TestEnumerationEnvironmentSettledBeforeDirCheck statically verifies that run
// resolves the permission check UID before it judges the hash directories it is about to write into.
//
// See sourceorder.AssertUIDResolvedBeforeCheck for why this reads the source
// instead of asserting on an execution.
func TestEnumerationEnvironmentSettledBeforeDirCheck(t *testing.T) {
	sourceorder.AssertUIDResolvedBeforeCheck(t, "main.go", "checkDirPermissions")
}
