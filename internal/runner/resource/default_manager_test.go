package resource

import (
	"context"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/stretchr/testify/assert"
)

func TestDefaultResourceManager_ModeDelegation(t *testing.T) {
	mockExec := &MockExecutor{}
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}

	mgr := NewDefaultResourceManager(mockExec, mockFS, mockPriv, ExecutionModeNormal, &DryRunOptions{})

	// Normal mode should delegate to normal manager's executor
	cmd := createTestCommand()
	env := map[string]string{"FOO": "BAR"}
	ctx := context.Background()
	expected := &executor.Result{ExitCode: 0, Stdout: "ok"}

	mockExec.On("Execute", ctx, cmd, env).Return(expected, nil)

	res, err := mgr.ExecuteCommand(ctx, cmd, createTestCommandGroup(), env)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.False(t, res.DryRun)

	// Switch to dry-run and verify delegation and analysis
	mgr.SetMode(ExecutionModeDryRun, &DryRunOptions{DetailLevel: DetailLevelDetailed})

	res2, err := mgr.ExecuteCommand(ctx, cmd, createTestCommandGroup(), env)
	assert.NoError(t, err)
	assert.NotNil(t, res2)
	assert.True(t, res2.DryRun)
	assert.NotNil(t, mgr.GetDryRunResults())
}

func TestDefaultResourceManager_TempDirDelegation(t *testing.T) {
	mockExec := &MockExecutor{}
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}

	mgr := NewDefaultResourceManager(mockExec, mockFS, mockPriv, ExecutionModeNormal, &DryRunOptions{})

	// Normal
	mockFS.On("CreateTempDir", "", "scr-group-").Return("/tmp/scr-group-123", nil)
	path, err := mgr.CreateTempDir("group")
	assert.NoError(t, err)
	assert.Contains(t, path, "/tmp/scr-")

	// Dry-run
	mgr.SetMode(ExecutionModeDryRun, &DryRunOptions{})
	path2, err := mgr.CreateTempDir("group")
	assert.NoError(t, err)
	assert.Contains(t, path2, "/tmp/scr-group-")
}

func TestDefaultResourceManager_PrivilegesAndNotifications(t *testing.T) {
	mockExec := &MockExecutor{}
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}

	mgr := NewDefaultResourceManager(mockExec, mockFS, mockPriv, ExecutionModeDryRun, &DryRunOptions{})

	// WithPrivileges should call provided fn in dry-run
	called := false
	err := mgr.WithPrivileges(context.Background(), func() error { called = true; return nil })
	assert.NoError(t, err)
	assert.True(t, called)

	// SendNotification should be no-op in normal and analysis in dry-run
	err = mgr.SendNotification("msg", map[string]interface{}{"k": "v"})
	assert.NoError(t, err)
}
