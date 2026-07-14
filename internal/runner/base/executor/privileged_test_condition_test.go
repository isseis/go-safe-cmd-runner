package executor_test

import (
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
)

// canRunPrivilegedIntegrationTest is the runtime half of the two-stage skip
// judgment for privileged integration tests: the //go:build integration tag
// keeps these tests out of ordinary builds entirely, and this pure function
// decides -- once the integration-tagged binary is actually running -- whether
// the current process has the root privilege a run-as test needs and whether
// the fixture user it is asked to switch to actually exists on this host. It
// takes no global state (euid, targetUser) so it can be tested here without
// any privileged environment or a real fixture user.
func canRunPrivilegedIntegrationTest(euid int, targetUser string) (ok bool, reason string) {
	if euid != 0 {
		return false, "privileged integration test requires running as root (euid 0)"
	}
	if targetUser == "" {
		return false, "privileged integration test requires TEST_RUNAS_TARGET_USER to name a fixture user"
	}
	if _, err := user.Lookup(targetUser); err != nil {
		return false, "target user " + targetUser + " does not exist on this host: " + err.Error()
	}
	return true, ""
}

func TestCanRunPrivilegedIntegrationTest(t *testing.T) {
	t.Run("not_root", func(t *testing.T) {
		ok, reason := canRunPrivilegedIntegrationTest(1000, "root")
		assert.False(t, ok)
		assert.NotEmpty(t, reason)
	})

	t.Run("no_target_user_configured", func(t *testing.T) {
		ok, reason := canRunPrivilegedIntegrationTest(0, "")
		assert.False(t, ok)
		assert.NotEmpty(t, reason)
	})

	t.Run("target_user_does_not_exist", func(t *testing.T) {
		ok, reason := canRunPrivilegedIntegrationTest(0, "no_such_user_0146_privileged_it")
		assert.False(t, ok)
		assert.NotEmpty(t, reason)
	})

	t.Run("conditions_satisfied", func(t *testing.T) {
		// "root" exists on every POSIX host this project targets, so this proves
		// the success path without requiring the test itself to run as root.
		ok, reason := canRunPrivilegedIntegrationTest(0, "root")
		assert.True(t, ok)
		assert.Empty(t, reason)
	})
}
