package resource

import (
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDefaultResourceManager_ModeDelegation(t *testing.T) {
	mockExec := executortesting.NewMockExecutor()
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}
	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback

	cmd := createTestCommand()
	env := map[string]string{"FOO": "BAR"}
	ctx := context.Background()

	t.Run("Normal Mode", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mockExec, mockFS, mockPriv, mockPathResolver, slog.Default(), ExecutionModeNormal, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		expected := &executor.Result{ExitCode: 0, Stdout: "ok"}

		mockExec.On("Execute", ctx, cmd, env, mock.Anything).Return(expected, nil)

		res, err := mgr.ExecuteCommand(ctx, cmd, createTestCommandGroup(), env)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.False(t, res.DryRun)
	})

	t.Run("Dry Run Mode", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mockExec, mockFS, mockPriv, mockPathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{DetailLevel: DetailLevelDetailed}, nil, 0)
		require.NoError(t, err)

		res2, err := mgr.ExecuteCommand(ctx, cmd, createTestCommandGroup(), env)
		assert.NoError(t, err)
		assert.NotNil(t, res2)
		assert.True(t, res2.DryRun)
		assert.NotNil(t, mgr.GetDryRunResults())
	})
}

func TestDefaultResourceManager_TempDirDelegation(t *testing.T) {
	mockExec := executortesting.NewMockExecutor()
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}
	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback

	t.Run("CreateTempDir Normal", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mockExec, mockFS, mockPriv, mockPathResolver, slog.Default(), ExecutionModeNormal, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)
		mockFS.On("CreateTempDir", "", "scr-group-").Return("/tmp/scr-group-123", nil)
		path, err := mgr.CreateTempDir("group")
		assert.NoError(t, err)
		assert.Contains(t, path, "/tmp/scr-")
	})

	t.Run("CreateTempDir Dry Run", func(t *testing.T) {
		// Dry-run
		mgr, err := NewDefaultResourceManager(mockExec, mockFS, mockPriv, mockPathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		path2, err := mgr.CreateTempDir("group")
		assert.NoError(t, err)
		assert.Contains(t, path2, "/tmp/scr-group-")
	})
}

func TestDefaultResourceManager_PrivilegesAndNotifications(t *testing.T) {
	mockExec := executortesting.NewMockExecutor()
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}
	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback

	mgr, err := NewDefaultResourceManager(mockExec, mockFS, mockPriv, mockPathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{}, nil, 0)
	require.NoError(t, err)

	// WithPrivileges should call provided fn in dry-run
	called := false
	err = mgr.WithPrivileges(context.Background(), func() error { called = true; return nil })
	assert.NoError(t, err)
	assert.True(t, called)

	// After WithPrivileges, a privilege analysis should be recorded
	res := mgr.GetDryRunResults()
	if assert.NotNil(t, res) {
		if assert.GreaterOrEqual(t, len(res.ResourceAnalyses), 1) {
			last := res.ResourceAnalyses[len(res.ResourceAnalyses)-1]
			assert.Equal(t, ResourceTypePrivilege, last.Type)
			assert.Equal(t, OperationEscalate, last.Operation)
			assert.Equal(t, "system_privileges", last.Target)
			// Parameters should include context of escalation
			assert.Equal(t, "privilege_escalation", last.Parameters["context"])
		}
	}
	prevLen := len(res.ResourceAnalyses)

	// SendNotification should be no-op in normal and analysis in dry-run
	err = mgr.SendNotification("msg", map[string]any{"k": "v"})
	assert.NoError(t, err)

	// After SendNotification, a network analysis should be recorded
	res2 := mgr.GetDryRunResults()
	if assert.NotNil(t, res2) {
		assert.Equal(t, prevLen+1, len(res2.ResourceAnalyses))
		last := res2.ResourceAnalyses[len(res2.ResourceAnalyses)-1]
		assert.Equal(t, ResourceTypeNetwork, last.Type)
		assert.Equal(t, OperationSend, last.Operation)
		assert.Equal(t, "notification_service", last.Target)
		// Parameters should include message and details
		assert.Equal(t, "msg", last.Parameters["message"])
		assert.Equal(t, map[string]any{"k": "v"}, last.Parameters["details"])
	}
}
