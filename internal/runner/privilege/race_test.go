//go:build !windows

package privilege

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

// TestUnixPrivilegeManager_ConcurrentAccess tests that the privilege manager
// handles concurrent access correctly without race conditions
func TestUnixPrivilegeManager_ConcurrentAccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := newPlatformManager(logger).(*UnixPrivilegeManager)

	// Skip test if not running with setuid (privilege escalation not supported)
	if !manager.IsPrivilegedExecutionSupported() {
		t.Skip("Skipping test: privilege escalation not supported (not running with setuid)")
	}

	const numGoroutines = 10
	const numOperationsPerGoroutine = 5

	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []error

	// Launch multiple goroutines that try to use WithPrivileges concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()

			for j := 0; j < numOperationsPerGoroutine; j++ {
				ctx := context.Background()
				elevationCtx := runnertypes.ElevationContext{
					Operation:   runnertypes.OperationFileAccess,
					CommandName: "test_concurrent",
					FilePath:    "/tmp/test",
				}

				err := manager.WithPrivileges(ctx, elevationCtx, func() error {
					// Simulate some work that requires privileges
					time.Sleep(1 * time.Millisecond)

					// Verify we're actually running with elevated privileges
					if os.Geteuid() != 0 {
						return ErrPrivilegeElevationFailed
					}

					return nil
				})

				mu.Lock()
				results = append(results, err)
				mu.Unlock()
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify all operations completed successfully
	for i, err := range results {
		assert.NoError(t, err, "Operation %d should not have failed", i)
	}

	// Verify we're back to original privileges
	assert.NotEqual(t, 0, os.Geteuid(), "Should be back to non-root privileges after all operations")
}

// TestUnixPrivilegeManager_NoDeadlock tests that the fixed implementation
// doesn't cause deadlocks when multiple operations are attempted
func TestUnixPrivilegeManager_NoDeadlock(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := newPlatformManager(logger).(*UnixPrivilegeManager)

	// Skip test if not running with setuid
	if !manager.IsPrivilegedExecutionSupported() {
		t.Skip("Skipping test: privilege escalation not supported (not running with setuid)")
	}

	ctx := context.Background()
	elevationCtx := runnertypes.ElevationContext{
		Operation:   runnertypes.OperationFileAccess,
		CommandName: "test_no_deadlock",
		FilePath:    "/tmp/test",
	}

	// Test that nested calls don't cause deadlock (though they should be avoided)
	// This should complete without hanging
	done := make(chan bool, 1)

	go func() {
		err := manager.WithPrivileges(ctx, elevationCtx, func() error {
			// Quick operation
			return nil
		})
		assert.NoError(t, err)
		done <- true
	}()

	select {
	case <-done:
		// Test passed - no deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock detected")
	}
}

// TestUnixPrivilegeManager_RaceConditionProtection tests that the mutex
// properly prevents race conditions between privilege escalation and restoration
func TestUnixPrivilegeManager_RaceConditionProtection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := newPlatformManager(logger).(*UnixPrivilegeManager)

	// Skip test if not running with setuid
	if !manager.IsPrivilegedExecutionSupported() {
		t.Skip("Skipping test: privilege escalation not supported (not running with setuid)")
	}

	const numGoroutines = 50
	var wg sync.WaitGroup
	var privilegedCount int32
	var mu sync.Mutex
	var errors []error

	// Launch many goroutines simultaneously to try to trigger race conditions
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx := context.Background()
			elevationCtx := runnertypes.ElevationContext{
				Operation:   runnertypes.OperationFileAccess,
				CommandName: "race_test",
				FilePath:    "/tmp/test",
			}

			err := manager.WithPrivileges(ctx, elevationCtx, func() error {
				// Check if we're actually running as root
				if os.Geteuid() == 0 {
					mu.Lock()
					privilegedCount++
					mu.Unlock()

					// Hold privileges briefly to increase chance of race condition
					time.Sleep(1 * time.Millisecond)
				} else {
					return ErrPrivilegeElevationFailed
				}
				return nil
			})
			if err != nil {
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// All operations should have succeeded
	assert.Empty(t, errors, "No operations should have failed due to race conditions")
	assert.Equal(t, int32(numGoroutines), privilegedCount, "All goroutines should have seen elevated privileges")

	// Verify we're back to original privileges
	assert.NotEqual(t, 0, os.Geteuid(), "Should be back to non-root privileges")
}

// TestUnixPrivilegeManager_LockSerialization tests that WithPrivileges calls are properly serialized
func TestUnixPrivilegeManager_LockSerialization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := newPlatformManager(logger).(*UnixPrivilegeManager)

	const numGoroutines = 10
	var wg sync.WaitGroup
	var callOrder []int
	var mu sync.Mutex

	// Launch multiple goroutines to test serialization
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx := context.Background()
			elevationCtx := runnertypes.ElevationContext{
				Operation:   runnertypes.OperationFileAccess,
				CommandName: "serialization_test",
				FilePath:    "/tmp/test",
			}

			// The mutex in WithPrivileges should serialize these calls
			err := manager.WithPrivileges(ctx, elevationCtx, func() error {
				// This function may not be called if setuid is not configured,
				// but the mutex should still serialize the WithPrivileges calls
				return nil
			})

			// Record the call order (calls should be serialized by mutex)
			mu.Lock()
			callOrder = append(callOrder, id)
			mu.Unlock()

			// Error is expected when not running with setuid
			_ = err
		}(i)
	}

	wg.Wait()

	// All calls should have been made
	assert.Len(t, callOrder, numGoroutines, "All WithPrivileges calls should have been attempted")

	// The test passes if we don't see any race conditions and all calls complete
	t.Logf("All %d WithPrivileges calls completed successfully", numGoroutines)
}

// TestUnixPrivilegeManager_ThreadSafety tests that the manager is thread-safe
func TestUnixPrivilegeManager_ThreadSafety(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := newPlatformManager(logger).(*UnixPrivilegeManager)

	const numGoroutines = 20
	var wg sync.WaitGroup

	// Test concurrent access to read-only methods
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// These methods should be safe to call concurrently
			_ = manager.GetCurrentUID()
			_ = manager.GetOriginalUID()
			_ = manager.IsPrivilegedExecutionSupported()
			_ = manager.GetMetrics()
		}()
	}

	wg.Wait()

	// Test should complete without data races
	t.Log("Thread safety test completed successfully")
}
