package executor_test

import (
	"fmt"
	"os"
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

// canRunSetuidModelIntegrationTest requires a non-root invoker and root euid.
func canRunSetuidModelIntegrationTest(uid, euid int, targetUser string) (bool, string) {
	if ok, reason := canRunPrivilegedIntegrationTest(euid, targetUser); !ok {
		return false, reason
	}
	if uid == 0 {
		return false, "setuid integration test requires a non-root real UID"
	}
	return true, ""
}

type skipper interface {
	Helper()
	Skipf(format string, args ...any)
}

// requireSetuidModel skips with the configured target even when identity fails first.
func requireSetuidModel(t skipper) {
	t.Helper()
	target := os.Getenv("TEST_RUNAS_TARGET_USER")
	if ok, reason := canRunSetuidModelIntegrationTest(os.Getuid(), os.Geteuid(), target); !ok {
		t.Skipf("%s (TEST_RUNAS_TARGET_USER=%q)", reason, target)
	}
}

type setuidSkipRecorder struct{ reason string }

func (*setuidSkipRecorder) Helper() {}
func (s *setuidSkipRecorder) Skipf(format string, args ...any) {
	s.reason = fmt.Sprintf(format, args...)
}

func TestCanRunSetuidModelIntegrationTest(t *testing.T) {
	for _, tc := range []struct {
		name      string
		uid, euid int
		target    string
		ok        bool
	}{
		{"real_uid_is_root", 0, 0, "root", false},
		{"not_root_euid", 1000, 1000, "root", false},
		{"no_target_user_configured", 1000, 0, "", false},
		{"target_user_does_not_exist", 1000, 0, "no_such_user_setuid_it", false},
		{"conditions_satisfied", 1000, 0, "root", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := canRunSetuidModelIntegrationTest(tc.uid, tc.euid, tc.target)
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.Empty(t, reason)
			} else {
				assert.NotEmpty(t, reason)
			}
		})
	}
}

func TestRequireSetuidModel_ReadsDocumentedEnvVar(t *testing.T) {
	const target = "no_such_user_setuid_wrapper_it"
	t.Setenv("TEST_RUNAS_TARGET_USER", target)
	recorder := &setuidSkipRecorder{}
	requireSetuidModel(recorder)
	assert.Contains(t, recorder.reason, target)
}
