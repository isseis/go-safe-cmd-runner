package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/user"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDryRunResourceManager_UserGroupValidation(t *testing.T) {
	t.Run("valid_user_group_specification", func(t *testing.T) {
		mockExec := executortestutil.NewMockExecutor()
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)

		manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
		require.NoError(t, err, "Failed to create DryRunResourceManager")
		manager.runAsResolver = risktypestestutil.StubResolveRunAsIdentSuccess

		cmd := executortestutil.CreateRuntimeCommand(
			"echo",
			[]string{"test"},
			executortestutil.WithName("test_user_group"),
			executortestutil.WithRunAsUser("testuser"),
			executortestutil.WithRunAsGroup("testgroup"),
		)

		group := &runnertypes.GroupSpec{
			Name:        "test_group",
			Description: "Test group",
		}

		_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// Check analysis contains user/group information
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Equal(t, "testuser", analysis.Parameters["run_as_user"].Value())
		assert.Equal(t, "testgroup", analysis.Parameters["run_as_group"].Value())
		assert.Contains(t, analysis.Impact.Description, "[INFO: User/Group identity resolution validated]")
	})

	t.Run("invalid_user_group_specification", func(t *testing.T) {
		mockExec := executortestutil.NewMockExecutor()
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
		require.NoError(t, err, "Failed to create DryRunResourceManager")
		manager.runAsResolver = risktypestestutil.StubResolveRunAsIdentUnknownUser

		cmd := executortestutil.CreateRuntimeCommand(
			"echo",
			[]string{"test"},
			executortestutil.WithName("test_invalid_user_group"),
			executortestutil.WithRunAsUser("nonexistent_user"),
			executortestutil.WithRunAsGroup("nonexistent_group"),
		)

		group := &runnertypes.GroupSpec{
			Name:        "test_group",
			Description: "Test group",
		}

		_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err) // Dry-run should not fail, but report issues
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// Check analysis contains error information
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Equal(t, "nonexistent_user", analysis.Parameters["run_as_user"].Value())
		assert.Equal(t, "nonexistent_group", analysis.Parameters["run_as_group"].Value())
		assert.Contains(t, analysis.Impact.Description, "[ERROR: User/Group identity resolution failed:")
		assert.Equal(t, riskLevelHigh, analysis.Impact.SecurityRisk)
	})

	t.Run("user_group_not_supported", func(t *testing.T) {
		mockExec := executortestutil.NewMockExecutor()
		mockPriv := privilegetestutil.NewMockPrivilegeManager(false) // Not supported

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
		require.NoError(t, err, "Failed to create DryRunResourceManager")
		manager.runAsResolver = risktypestestutil.StubResolveRunAsIdentSuccess

		cmd := executortestutil.CreateRuntimeCommand(
			"echo",
			[]string{"test"},
			executortestutil.WithName("test_user_group_unsupported"),
			executortestutil.WithRunAsUser("testuser"),
			executortestutil.WithRunAsGroup("testgroup"),
		)

		group := &runnertypes.GroupSpec{
			Name:        "test_group",
			Description: "Test group",
		}

		_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// The resolution is still validated even though privileged execution is
		// unsupported; both the validation result and the unsupported warning appear.
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Contains(t, analysis.Impact.Description, "[INFO: User/Group identity resolution validated]")
		assert.Contains(t, analysis.Impact.Description, "[WARNING: User/Group privilege management not supported]")
		assert.NotContains(t, analysis.Impact.Description, "[ERROR:")
		assert.Equal(t, runnertypes.RiskLevelLow.String(), analysis.Impact.SecurityRisk)
	})

	t.Run("no_privilege_manager", func(t *testing.T) {
		mockExec := executortestutil.NewMockExecutor()
		// No privilege manager provided

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager, err := NewDryRunResourceManager(mockExec, nil, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
		require.NoError(t, err, "Failed to create DryRunResourceManager")
		manager.runAsResolver = risktypestestutil.StubResolveRunAsIdentSuccess

		cmd := executortestutil.CreateRuntimeCommand(
			"echo",
			[]string{"test"},
			executortestutil.WithName("test_no_privmgr"),
			executortestutil.WithRunAsUser("testuser"),
			executortestutil.WithRunAsGroup("testgroup"),
		)

		group := &runnertypes.GroupSpec{
			Name:        "test_group",
			Description: "Test group",
		}

		_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// The resolution is still validated even with no privilege manager at
		// all; both the validation result and the unsupported warning appear.
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Contains(t, analysis.Impact.Description, "[INFO: User/Group identity resolution validated]")
		assert.Contains(t, analysis.Impact.Description, "[WARNING: User/Group privilege management not supported]")
		assert.NotContains(t, analysis.Impact.Description, "[ERROR:")
		assert.Equal(t, runnertypes.RiskLevelLow.String(), analysis.Impact.SecurityRisk)
	})

	t.Run("only_user_specified", func(t *testing.T) {
		mockExec := executortestutil.NewMockExecutor()
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
		require.NoError(t, err, "Failed to create DryRunResourceManager")

		var gotGroupName string
		manager.runAsResolver = func(base risktypes.RunAsIdent, userName, groupName string) (risktypes.RunAsIdent, error) {
			gotGroupName = groupName
			return risktypestestutil.StubResolveRunAsIdentSuccess(base, userName, groupName)
		}

		cmd := executortestutil.CreateRuntimeCommand(
			"echo",
			[]string{"test"},
			executortestutil.WithName("test_user_only"),
			executortestutil.WithRunAsUser("testuser"),
		)

		group := &runnertypes.GroupSpec{
			Name:        "test_group",
			Description: "Test group",
		}

		_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// The resolver was invoked with an empty group name.
		assert.Empty(t, gotGroupName)

		// Check analysis
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		assert.Equal(t, "testuser", analysis.Parameters["run_as_user"].Value())
		assert.Equal(t, "", analysis.Parameters["run_as_group"].Value())
		assert.Contains(t, analysis.Impact.Description, "[INFO: User/Group identity resolution validated]")
	})

	t.Run("no_user_group_specification", func(t *testing.T) {
		mockExec := executortestutil.NewMockExecutor()
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)

		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
		manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
		require.NoError(t, err, "Failed to create DryRunResourceManager")

		resolverCalled := false
		manager.runAsResolver = func(base risktypes.RunAsIdent, userName, groupName string) (risktypes.RunAsIdent, error) {
			resolverCalled = true
			return risktypestestutil.StubResolveRunAsIdentSuccess(base, userName, groupName)
		}

		cmd := executortestutil.CreateRuntimeCommand(
			"echo",
			[]string{"test"},
			executortestutil.WithName("test_no_user_group"),
		)

		group := &runnertypes.GroupSpec{
			Name:        "test_group",
			Description: "Test group",
		}

		_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.DryRun)

		// Should not call the run-as resolver.
		assert.False(t, resolverCalled)

		// Check analysis does not contain user/group info
		analysis := result.Analysis
		assert.NotNil(t, analysis)
		_, hasRunAsUser := analysis.Parameters["run_as_user"]
		_, hasRunAsGroup := analysis.Parameters["run_as_group"]
		assert.False(t, hasRunAsUser)
		assert.False(t, hasRunAsGroup)
		assert.NotContains(t, analysis.Impact.Description, "User/Group")
	})
}

// TestDryRunResourceManager_GroupNameResolutionFailure covers a
// resolvable-user/unresolvable-group configuration: it is reported as a
// validation failure with SecurityRisk raised to "high".
func TestDryRunResourceManager_GroupNameResolutionFailure(t *testing.T) {
	mockExec := executortestutil.NewMockExecutor()
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil)

	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
	require.NoError(t, err)
	manager.runAsResolver = risktypestestutil.StubResolveRunAsIdentUnknownGroup

	cmd := executortestutil.CreateRuntimeCommand(
		"echo",
		[]string{"test"},
		executortestutil.WithName("test_group_unknown"),
		executortestutil.WithRunAsUser("testuser"),
		executortestutil.WithRunAsGroup("nonexistent_group"),
	)
	group := &runnertypes.GroupSpec{Name: "test_group"}

	_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})
	require.NoError(t, err)

	analysis := result.Analysis
	assert.Contains(t, analysis.Impact.Description, "[ERROR: User/Group identity resolution failed:")
	assert.Equal(t, riskLevelHigh, analysis.Impact.SecurityRisk)
}

// TestDryRunResourceManager_SupplementaryGroupsUnavailable covers a
// configuration where the user/group names resolve but supplementary group
// enumeration fails (Groups == nil): it is reported as a validation failure.
// Before this task, dry-run user/group resolution never enumerated
// supplementary groups, so this exact input was reported as "validated".
func TestDryRunResourceManager_SupplementaryGroupsUnavailable(t *testing.T) {
	mockExec := executortestutil.NewMockExecutor()
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil)

	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
	require.NoError(t, err)
	manager.runAsResolver = risktypestestutil.StubResolveRunAsIdentNilGroups

	cmd := executortestutil.CreateRuntimeCommand(
		"echo",
		[]string{"test"},
		executortestutil.WithName("test_groups_unavailable"),
		executortestutil.WithRunAsUser("testuser"),
		executortestutil.WithRunAsGroup("testgroup"),
	)
	group := &runnertypes.GroupSpec{Name: "test_group"}

	_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})
	require.NoError(t, err)

	analysis := result.Analysis
	assert.Contains(t, analysis.Impact.Description, "[ERROR: User/Group identity resolution failed:")
	assert.Equal(t, riskLevelHigh, analysis.Impact.SecurityRisk)
}

// TestDryRunResourceManager_RiskRaiseIsMonotonic covers the case where the
// command's effective risk is already "critical" and run-as identity
// resolution also fails: SecurityRisk must stay "critical", not fall to
// "high".
func TestDryRunResourceManager_RiskRaiseIsMonotonic(t *testing.T) {
	mockExec := executortestutil.NewMockExecutor()
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil)

	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, riskLevelTestEvaluator{level: runnertypes.RiskLevelCritical}, nil)
	require.NoError(t, err)
	manager.runAsResolver = risktypestestutil.StubResolveRunAsIdentUnknownUser

	cmd := executortestutil.CreateRuntimeCommand(
		"echo",
		[]string{"test"},
		executortestutil.WithName("test_monotonic_risk"),
		executortestutil.WithRunAsUser("nonexistent_user"),
	)
	group := &runnertypes.GroupSpec{Name: "test_group"}

	_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})
	require.NoError(t, err)

	assert.Equal(t, runnertypes.RiskLevelCritical.String(), result.Analysis.Impact.SecurityRisk)
}

// TestDryRunResourceManager_ResolverCalledOncePerCommand covers a single
// command with a run_as specification: the injected resolver is invoked
// exactly once.
func TestDryRunResourceManager_ResolverCalledOncePerCommand(t *testing.T) {
	mockExec := executortestutil.NewMockExecutor()
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil)

	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
	require.NoError(t, err)

	calls := 0
	manager.runAsResolver = func(base risktypes.RunAsIdent, userName, groupName string) (risktypes.RunAsIdent, error) {
		calls++
		return risktypestestutil.StubResolveRunAsIdentSuccess(base, userName, groupName)
	}

	cmd := executortestutil.CreateRuntimeCommand(
		"echo",
		[]string{"test"},
		executortestutil.WithName("test_resolver_once"),
		executortestutil.WithRunAsUser("testuser"),
		executortestutil.WithRunAsGroup("testgroup"),
	)
	group := &runnertypes.GroupSpec{Name: "test_group"}

	_, _, err = manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
}

// TestDryRunResourceManager_RunAsIdentityLogAttributes covers the structured
// log record: it carries the expected attributes as individual JSON keys
// (not folded into a single message string), and never leaks an environment
// variable value the command was given.
func TestDryRunResourceManager_RunAsIdentityLogAttributes(t *testing.T) {
	const secretEnvValue = "sentinel-env-value-0158"

	run := func(t *testing.T, resolver risktypes.RunAsResolver) map[string]any {
		t.Helper()

		mockExec := executortestutil.NewMockExecutor()
		mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
		mockPathResolver := &MockPathResolver{}
		setupStandardCommandPaths(mockPathResolver)
		mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil)

		manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
		require.NoError(t, err)
		manager.runAsResolver = resolver

		var buf bytes.Buffer
		manager.logger = slog.New(slog.NewJSONHandler(&buf, nil))

		cmd := executortestutil.CreateRuntimeCommand(
			"echo",
			[]string{"test"},
			executortestutil.WithName("test_log_attrs"),
			executortestutil.WithRunAsUser("testuser"),
			executortestutil.WithRunAsGroup("testgroup"),
		)
		group := &runnertypes.GroupSpec{Name: "test_group"}
		env := map[string]string{"GSCR_TEST_SECRET": secretEnvValue}

		_, _, err = manager.ExecuteCommand(context.Background(), cmd, group, env)
		require.NoError(t, err)

		output := buf.String()
		assert.NotContains(t, output, secretEnvValue, "structured log must not leak environment variable values")

		lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
		require.Len(t, lines, 1, "exactly one log record must be emitted per validated command")

		var record map[string]any
		require.NoError(t, json.Unmarshal(lines[0], &record))
		return record
	}

	t.Run("success", func(t *testing.T) {
		record := run(t, risktypestestutil.StubResolveRunAsIdentSuccess)
		for _, key := range []string{"dry_run", "command", "group", "run_as_user", "run_as_group", "resolved_uid", "resolved_gid"} {
			assert.Contains(t, record, key)
		}
		assert.Equal(t, true, record["dry_run"])
		assert.Equal(t, "test_log_attrs", record["command"])
		assert.Equal(t, "test_group", record["group"])
		assert.Equal(t, "testuser", record["run_as_user"])
		assert.Equal(t, "testgroup", record["run_as_group"])
	})

	t.Run("failure", func(t *testing.T) {
		record := run(t, risktypestestutil.StubResolveRunAsIdentUnknownUser)
		for _, key := range []string{"dry_run", "command", "group", "run_as_user", "run_as_group", "failure_kind", "error"} {
			assert.Contains(t, record, key)
		}
		assert.Equal(t, "user_unknown", record["failure_kind"])
	})
}

// TestRunAsFailureKind covers the runAsFailureKind classification, including
// the base-identity variant that distinguishes a failure originating from
// OriginalExecutionIdentity() itself (run_as_group-only, base.Groups == nil)
// from an ordinary supplementary-group enumeration failure for the named
// user.
func TestRunAsFailureKind(t *testing.T) {
	tests := map[string]struct {
		err      error
		userName string
		base     risktypes.RunAsIdent
		want     string
	}{
		"user_unknown": {
			err:  user.UnknownUserError("nonexistent_user"),
			want: "user_unknown",
		},
		"group_unknown": {
			err:  user.UnknownGroupError("nonexistent_group"),
			want: "group_unknown",
		},
		"supplementary_groups_unavailable": {
			err:      risktypes.ErrRunAsSupplementaryGroupsUnavailable,
			userName: "testuser",
			base:     risktypes.RunAsIdent{Groups: []uint32{1000}},
			want:     "supplementary_groups_unavailable",
		},
		"base_identity_groups_unavailable": {
			err:      risktypes.ErrRunAsSupplementaryGroupsUnavailable,
			userName: "",
			base:     risktypes.RunAsIdent{Groups: nil},
			want:     "base_identity_groups_unavailable",
		},
		"lookup_error": {
			err:  os.ErrDeadlineExceeded,
			want: "lookup_error",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, runAsFailureKind(tt.err, tt.userName, tt.base))
		})
	}
}

// TestParseDisplayRiskLevel covers the 5 canonical strings plus the empty and
// unknown-string cases.
func TestParseDisplayRiskLevel(t *testing.T) {
	tests := map[string]runnertypes.RiskLevel{
		"low":         runnertypes.RiskLevelLow,
		"medium":      runnertypes.RiskLevelMedium,
		"high":        runnertypes.RiskLevelHigh,
		"critical":    runnertypes.RiskLevelCritical,
		"unknown":     runnertypes.RiskLevelUnknown,
		"":            runnertypes.RiskLevelUnknown,
		"not-a-level": runnertypes.RiskLevelUnknown,
	}
	for s, want := range tests {
		t.Run(s, func(t *testing.T) {
			assert.Equal(t, want, parseDisplayRiskLevel(s))
		})
	}
}

// TestDryRunPreservesProcessIdentity covers the dry-run side-effect
// contract: dry-running a command with a run_as specification must not
// change the process's own UID, GID, or supplementary groups. This test is
// added ahead of the phase that removes the equivalent guard in the
// privilege package (TestWithPrivileges_UserGroupDryRunDoesNotChangeIdentity),
// so the invariant is never left unchecked.
func TestDryRunPreservesProcessIdentity(t *testing.T) {
	beforeUID := os.Getuid()
	beforeGID := os.Getgid()
	beforeGroups, groupsErr := os.Getgroups()

	mockExec := executortestutil.NewMockExecutor()
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil)

	manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
	require.NoError(t, err)
	manager.runAsResolver = risktypestestutil.StubResolveRunAsIdentSuccess

	cmd := executortestutil.CreateRuntimeCommand(
		"echo",
		[]string{"test"},
		executortestutil.WithName("test_identity_preserved"),
		executortestutil.WithRunAsUser("testuser"),
		executortestutil.WithRunAsGroup("testgroup"),
	)
	group := &runnertypes.GroupSpec{Name: "test_group"}

	_, _, err = manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})
	require.NoError(t, err)

	assert.Equal(t, beforeUID, os.Getuid())
	assert.Equal(t, beforeGID, os.Getgid())
	afterGroups, afterErr := os.Getgroups()
	require.Equal(t, groupsErr == nil, afterErr == nil)
	if groupsErr == nil {
		assert.ElementsMatch(t, beforeGroups, afterGroups)
	}
}

// TestDryRunResourceManager_SharedResolutionCases reads the standard
// resolution-case table (risktypestestutil.RunAsResolutionCases) and verifies
// that, for each case, the dry-run manager's fail-closed judgment agrees with
// what the table expects (WantFailure). The test checks only resolution
// consent: the individual sub-tests above cover the exact message text,
// risk-level values, and log attributes. What this test proves is that the
// dry-run manager rejects exactly the same inputs the table marks as failing.
func TestDryRunResourceManager_SharedResolutionCases(t *testing.T) {
	for _, tc := range risktypestestutil.RunAsResolutionCases() {
		t.Run(tc.Name, func(t *testing.T) {
			mockExec := executortestutil.NewMockExecutor()
			mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
			mockPathResolver := &MockPathResolver{}
			setupStandardCommandPaths(mockPathResolver)
			mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil)

			manager, err := NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, &DryRunOptions{}, permissiveTestEvaluator{}, nil)
			require.NoError(t, err)
			manager.runAsResolver = tc.Resolver

			cmd := executortestutil.CreateRuntimeCommand(
				"echo",
				[]string{"test"},
				executortestutil.WithName("test_shared_"+tc.Name),
				executortestutil.WithRunAsUser(tc.UserName),
				executortestutil.WithRunAsGroup(tc.GroupName),
			)
			group := &runnertypes.GroupSpec{Name: "test_group"}

			_, result, err := manager.ExecuteCommand(context.Background(), cmd, group, map[string]string{})
			require.NoError(t, err)
			require.NotNil(t, result)

			analysis := result.Analysis
			require.NotNil(t, analysis)

			if tc.WantFailure {
				assert.Contains(t, analysis.Impact.Description, "identity resolution failed")
				assert.GreaterOrEqual(t, parseDisplayRiskLevel(analysis.Impact.SecurityRisk), runnertypes.RiskLevelHigh,
					"validation failure must raise SecurityRisk to at least high")
			} else {
				assert.Contains(t, analysis.Impact.Description, "identity resolution validated")
			}
		})
	}
}
