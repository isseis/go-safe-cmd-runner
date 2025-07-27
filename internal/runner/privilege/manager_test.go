package privilege

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestManager_Interface(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	// Test interface implementation
	assert.NotNil(t, manager)
	assert.Implements(t, (*Manager)(nil), manager)
}

func TestElevationContext(t *testing.T) {
	ctx := runnertypes.ElevationContext{
		Operation:   runnertypes.OperationHealthCheck,
		CommandName: "test",
		FilePath:    "/test/path",
		StartTime:   time.Now(),
		OriginalUID: 1000,
		TargetUID:   0,
	}

	assert.Equal(t, runnertypes.OperationHealthCheck, ctx.Operation)
	assert.Equal(t, "test", ctx.CommandName)
	assert.Equal(t, "/test/path", ctx.FilePath)
	assert.Equal(t, 1000, ctx.OriginalUID)
	assert.Equal(t, 0, ctx.TargetUID)
}

func TestPrivilegeError(t *testing.T) {
	err := &Error{
		Operation:   runnertypes.OperationCommandExecution,
		CommandName: "test_cmd",
		OriginalUID: 1000,
		TargetUID:   0,
		SyscallErr:  ErrPrivilegeElevationFailed,
		Timestamp:   time.Now(),
	}

	expectedMsg := "privilege operation 'command_execution' failed for command 'test_cmd' (uid 1000->0): failed to elevate privileges"
	assert.Equal(t, expectedMsg, err.Error())
	assert.Equal(t, ErrPrivilegeElevationFailed, err.Unwrap())
}

func TestOperationConstants(t *testing.T) {
	operations := []runnertypes.Operation{
		runnertypes.OperationFileHashCalculation,
		runnertypes.OperationCommandExecution,
		runnertypes.OperationFileAccess,
		runnertypes.OperationHealthCheck,
	}

	expected := []string{
		"file_hash_calculation",
		"command_execution",
		"file_access",
		"health_check",
	}

	for i, op := range operations {
		assert.Equal(t, expected[i], string(op))
	}
}

func TestManager_BasicFunctionality(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	// Test GetOriginalUID returns reasonable value
	originalUID := manager.GetOriginalUID()
	assert.GreaterOrEqual(t, originalUID, -1) // -1 for Windows, >= 0 for Unix

	// Test GetCurrentUID returns reasonable value
	currentUID := manager.GetCurrentUID()
	assert.GreaterOrEqual(t, currentUID, -1) // -1 for Windows, >= 0 for Unix
}

func TestManager_WithPrivileges_UnsupportedPlatform(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	// This test assumes we're running without setuid in normal test environment
	if manager.IsPrivilegedExecutionSupported() {
		t.Skip("Test environment has privileged execution enabled")
	}

	ctx := context.Background()
	elevationCtx := runnertypes.ElevationContext{
		Operation:   runnertypes.OperationHealthCheck,
		CommandName: "test",
	}

	err := manager.WithPrivileges(ctx, elevationCtx, func() error {
		return nil
	})

	// Should fail because setuid is not configured in test environment
	assert.Error(t, err)
}

func TestManager_HealthCheck(t *testing.T) {
	logger := slog.Default()
	manager := NewManager(logger)

	ctx := context.Background()
	err := manager.HealthCheck(ctx)

	if manager.IsPrivilegedExecutionSupported() {
		// If supported, health check should pass
		assert.NoError(t, err)
	} else {
		// If not supported, should return appropriate error
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrPrivilegedExecutionNotAvailable)
	}
}
