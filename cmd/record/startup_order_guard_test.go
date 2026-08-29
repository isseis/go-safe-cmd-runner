//go:build test

package main

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/sourceorder"
	"github.com/stretchr/testify/assert"
)

// TestEnumerationEnvironmentSettledBeforeDirCheck statically verifies that run
// resolves the permission check UID before it judges the hash directories it is about to write into.
//
// EnsurePermissionCheckUID is also what settles the group-enumeration
// completeness classification for the process, and that classification is
// settled at startup or not at all: an enumeration reached before it denies.
// The order therefore decides whether the warning about a host this build
// cannot enumerate precedes the first denial over it. Getting it right leaves
// no runtime trace, and every execution test in this package injects a stub
// through deps, so the guard reads the source.
func TestEnumerationEnvironmentSettledBeforeDirCheck(t *testing.T) {
	t.Run("run resolves the permission check UID before checking directories", func(t *testing.T) {
		body := sourceorder.Body(t, "main.go", "run")

		ensure := sourceorder.OnlyRef(t, body, "EnsurePermissionCheckUID")
		check := sourceorder.OnlyRef(t, body, "checkDirPermissions")

		assert.Less(t, ensure, check,
			"run must resolve the permission check UID, and with it settle the enumeration completeness classification, before the first decision that depends on it")
	})

	t.Run("control: the order assertion fails on reordered source", func(t *testing.T) {
		reordered := "package main\n" +
			"func run() int {\n" +
			"\t_, _ = checkDirPermissions(nil, deps{}, nil)\n" +
			"\tensureUID := groupmembership.New().EnsurePermissionCheckUID\n" +
			"\t_ = ensureUID()\n" +
			"\treturn 0\n" +
			"}\n"

		body := sourceorder.BodyInSource(t, "reordered.go", reordered, "run")

		ensure := sourceorder.OnlyRef(t, body, "EnsurePermissionCheckUID")
		check := sourceorder.OnlyRef(t, body, "checkDirPermissions")
		assert.Greater(t, ensure, check,
			"the scan must see the reordered source inside run's body, not merely find both references")
	})
}
