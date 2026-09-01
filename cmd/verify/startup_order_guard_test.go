package main

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/sourceorder"
)

// TestEnumerationEnvironmentSettledBeforeDirCheck statically verifies that run
// resolves the permission check UID before it judges the hash directory it is about to read.
//
// See sourceorder.AssertUIDResolvedBeforeCheck for why this reads the source
// instead of asserting on an execution.
func TestEnumerationEnvironmentSettledBeforeDirCheck(t *testing.T) {
	sourceorder.AssertUIDResolvedBeforeCheck(t, "main.go", "checkHashDirPermissions")
}
