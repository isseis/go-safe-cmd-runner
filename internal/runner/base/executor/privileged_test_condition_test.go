package executor_test

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	return canRunSetuidModelIntegrationTestWithLookup(uid, euid, targetUser, user.Lookup)
}

type userLookupFunc func(string) (*user.User, error)

func canRunSetuidModelIntegrationTestWithLookup(uid, euid int, targetUser string, lookup userLookupFunc) (bool, string) {
	if euid != 0 {
		return false, "privileged integration test requires running as root (euid 0)"
	}
	if targetUser == "" {
		return false, "privileged integration test requires TEST_RUNAS_TARGET_USER to name a fixture user"
	}
	if _, err := lookup(targetUser); err != nil {
		return false, "target user " + targetUser + " does not exist on this host: " + err.Error()
	}
	if uid == 0 {
		return false, "setuid integration test requires a non-root real UID"
	}
	return true, ""
}

type setuidModelProbe struct {
	uid        func() int
	euid       func() int
	targetUser func() string
	lookupUser userLookupFunc
}

var osSetuidModelProbe = setuidModelProbe{
	uid:        os.Getuid,
	euid:       os.Geteuid,
	targetUser: func() string { return os.Getenv("TEST_RUNAS_TARGET_USER") },
	lookupUser: user.Lookup,
}

type skipper interface {
	Helper()
	Skipf(format string, args ...any)
}

// requireSetuidModel skips unless every prerequisite of the setuid model is present.
func requireSetuidModel(t skipper) {
	requireSetuidModelWithProbe(t, osSetuidModelProbe)
}

func requireSetuidModelWithProbe(t skipper, probe setuidModelProbe) {
	t.Helper()
	target := probe.targetUser()
	if ok, reason := canRunSetuidModelIntegrationTestWithLookup(
		probe.uid(), probe.euid(), target, probe.lookupUser,
	); !ok {
		t.Skipf("%s (TEST_RUNAS_TARGET_USER=%q)", reason, target)
		return
	}
}

type setuidSkipRecorder struct {
	reasons []string
}

func (*setuidSkipRecorder) Helper() {}
func (s *setuidSkipRecorder) Skipf(format string, args ...any) {
	s.reasons = append(s.reasons, fmt.Sprintf(format, args...))
}

func (s *setuidSkipRecorder) reason() string {
	if len(s.reasons) == 0 {
		return ""
	}
	return s.reasons[0]
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
	require.Len(t, recorder.reasons, 1)
	assert.Contains(t, recorder.reason(), target)
}

func TestRequireSetuidModel_SupportedDoesNotSkip(t *testing.T) {
	probe := setuidModelProbe{
		uid:        func() int { return 1000 },
		euid:       func() int { return 0 },
		targetUser: func() string { return "fixture" },
		lookupUser: func(string) (*user.User, error) { return &user.User{Username: "fixture"}, nil },
	}
	recorder := &setuidSkipRecorder{}
	requireSetuidModelWithProbe(recorder, probe)
	assert.Empty(t, recorder.reasons)
}

func TestRequireSetuidModel_UnsupportedSkipsExactlyOnce(t *testing.T) {
	errLookup := errors.New("lookup failed")
	tests := []struct {
		name       string
		uid        int
		euid       int
		target     string
		lookupErr  error
		wantReason string
	}{
		{name: "real_uid_is_root", uid: 0, euid: 0, target: "fixture", wantReason: "non-root real UID"},
		{name: "effective_uid_is_not_root", uid: 1000, euid: 1000, target: "fixture", wantReason: "euid 0"},
		{name: "target_is_missing", uid: 1000, euid: 0, wantReason: "TEST_RUNAS_TARGET_USER"},
		{name: "target_lookup_fails", uid: 1000, euid: 0, target: "fixture", lookupErr: errLookup, wantReason: "lookup failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := setuidModelProbe{
				uid:        func() int { return tt.uid },
				euid:       func() int { return tt.euid },
				targetUser: func() string { return tt.target },
				lookupUser: func(string) (*user.User, error) {
					if tt.lookupErr != nil {
						return nil, tt.lookupErr
					}
					return &user.User{Username: tt.target}, nil
				},
			}
			recorder := &setuidSkipRecorder{}
			requireSetuidModelWithProbe(recorder, probe)
			require.Len(t, recorder.reasons, 1)
			assert.Contains(t, recorder.reason(), tt.wantReason)
		})
	}
}
