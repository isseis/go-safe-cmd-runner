package resource

import (
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// testMocks holds all the mocks needed for testing DefaultResourceManager
type testMocks struct {
	exec         *executortesting.MockExecutor
	fs           *MockFileSystem
	priv         *MockPrivilegeManager
	pathResolver *MockPathResolver
}

// setupTestMocks creates and initializes all mocks for DefaultResourceManager tests
func setupTestMocks() *testMocks {
	mockExec := executortesting.NewMockExecutor()
	mockFS := &MockFileSystem{}
	mockPriv := &MockPrivilegeManager{}
	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil)

	return &testMocks{
		exec:         mockExec,
		fs:           mockFS,
		priv:         mockPriv,
		pathResolver: mockPathResolver,
	}
}

func TestDefaultResourceManager_ModeDelegation(t *testing.T) {
	mocks := setupTestMocks()

	cmd := executortesting.CreateRuntimeCommand("echo", []string{})
	env := map[string]string{"FOO": "BAR"}
	ctx := context.Background()

	t.Run("Normal Mode", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeNormal, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		expected := &executor.Result{ExitCode: 0, Stdout: "ok"}

		mocks.exec.On("Execute", ctx, cmd, env, mock.Anything).Return(expected, nil)

		_, res, err := mgr.ExecuteCommand(ctx, cmd, createTestCommandGroup(), env)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.False(t, res.DryRun)
	})

	t.Run("Dry Run Mode", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{DetailLevel: DetailLevelDetailed}, nil, 0)
		require.NoError(t, err)

		_, res2, err := mgr.ExecuteCommand(ctx, cmd, createTestCommandGroup(), env)
		assert.NoError(t, err)
		assert.NotNil(t, res2)
		assert.True(t, res2.DryRun)
		assert.NotNil(t, mgr.GetDryRunResults())
	})
}

func TestDefaultResourceManager_TempDirDelegation(t *testing.T) {
	mocks := setupTestMocks()

	t.Run("CreateTempDir Normal", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeNormal, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)
		mocks.fs.On("CreateTempDir", "", "scr-group-").Return("/tmp/scr-group-123", nil)
		path, err := mgr.CreateTempDir("group")
		assert.NoError(t, err)
		assert.Contains(t, path, "/tmp/scr-")
	})

	t.Run("CreateTempDir Dry Run", func(t *testing.T) {
		// Dry-run
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		path2, err := mgr.CreateTempDir("group")
		assert.NoError(t, err)
		assert.Contains(t, path2, "/tmp/scr-group-")
	})
}

func TestDefaultResourceManager_PrivilegesAndNotifications(t *testing.T) {
	mocks := setupTestMocks()

	mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{}, nil, 0)
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
			assert.Equal(t, TypePrivilege, last.Type)
			assert.Equal(t, OperationEscalate, last.Operation)
			assert.Equal(t, "system_privileges", last.Target)
			// Parameters should include context of escalation
			assert.Equal(t, "privilege_escalation", last.Parameters["context"].Value())
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
		assert.Equal(t, TypeNetwork, last.Type)
		assert.Equal(t, OperationSend, last.Operation)
		assert.Equal(t, "notification_service", last.Target)
		// Parameters should include message and details
		assert.Equal(t, "msg", last.Parameters["message"].Value())
		assert.Equal(t, map[string]any{"k": "v"}, last.Parameters["details"].Value())
	}
}

func TestDefaultResourceManager_CleanupTempDir(t *testing.T) {
	mocks := setupTestMocks()

	t.Run("Dry Run Mode", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		// Dry-run should not actually remove but should delegate without error
		err = mgr.CleanupTempDir("/tmp/scr-test-123")
		assert.NoError(t, err)
	})
}

func TestDefaultResourceManager_CleanupAllTempDirs(t *testing.T) {
	mocks := setupTestMocks()

	t.Run("Dry Run Mode", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		err = mgr.CleanupAllTempDirs()
		assert.NoError(t, err)
	})
}

func TestDefaultResourceManager_RecordAnalysis(t *testing.T) {
	mocks := setupTestMocks()

	t.Run("Normal Mode - No-op", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeNormal, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		analysis := &Analysis{
			Type:      TypeFilesystem,
			Operation: OperationCreate,
			Target:    "/test/file.txt",
		}

		// Should be no-op in normal mode
		mgr.RecordAnalysis(analysis)

		// GetDryRunResults should return nil in normal mode
		assert.Nil(t, mgr.GetDryRunResults())
	})

	t.Run("Dry Run Mode - Records Analysis", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		analysis := &Analysis{
			Type:      TypeFilesystem,
			Operation: OperationCreate,
			Target:    "/test/output.txt",
		}

		mgr.RecordAnalysis(analysis)

		// Should have recorded the analysis
		results := mgr.GetDryRunResults()
		require.NotNil(t, results)
		assert.Greater(t, len(results.ResourceAnalyses), 0, "Should have at least one analysis")
	})
}

func TestDefaultResourceManager_RecordGroupAnalysis(t *testing.T) {
	mocks := setupTestMocks()

	debugInfo := &DebugInfo{}

	t.Run("Normal Mode", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeNormal, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		err = mgr.RecordGroupAnalysis("test-group", debugInfo)
		assert.NoError(t, err)
	})

	t.Run("Dry Run Mode", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		err = mgr.RecordGroupAnalysis("test-group", debugInfo)
		assert.NoError(t, err)
	})
}

func TestDefaultResourceManager_UpdateCommandDebugInfo(t *testing.T) {
	mocks := setupTestMocks()

	debugInfo := &DebugInfo{}

	t.Run("Normal Mode", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeNormal, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		token := CommandToken("test-token-123")
		// This may return an error for invalid token, which is expected behavior
		// The implementation may be a no-op in normal mode
		_ = mgr.UpdateCommandDebugInfo(token, debugInfo)
	})

	t.Run("Dry Run Mode - execution creates token", func(t *testing.T) {
		mgr, err := NewDefaultResourceManager(mocks.exec, mocks.fs, mocks.priv, mocks.pathResolver, slog.Default(), ExecutionModeDryRun, &DryRunOptions{}, nil, 0)
		require.NoError(t, err)

		// Execute a command to get a valid token
		cmd := executortesting.CreateRuntimeCommand("echo", []string{})
		env := map[string]string{}
		ctx := context.Background()

		token, _, err := mgr.ExecuteCommand(ctx, cmd, createTestCommandGroup(), env)
		require.NoError(t, err)

		// Now update debug info with the valid token
		err = mgr.UpdateCommandDebugInfo(token, debugInfo)
		assert.NoError(t, err)
	})
}
