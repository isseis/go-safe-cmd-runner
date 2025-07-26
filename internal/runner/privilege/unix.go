//go:build !windows

package privilege

import (
	"context"
	"fmt"
	"log/slog"
	"log/syslog"
	"os"
	"sync"
	"syscall"
	"time"
)

// UnixPrivilegeManager implements privilege management for Unix systems using setuid
type UnixPrivilegeManager struct {
	logger      *slog.Logger
	originalUID int
	originalGID int
	isSetuid    bool
	metrics     Metrics
	mu          sync.RWMutex
}

func newPlatformManager(logger *slog.Logger) Manager {
	originalUID := syscall.Getuid()
	effectiveUID := syscall.Geteuid()

	return &UnixPrivilegeManager{
		logger:      logger,
		originalUID: originalUID,
		originalGID: syscall.Getgid(),
		isSetuid:    effectiveUID == 0 && originalUID != 0,
	}
}

// WithPrivileges executes a function with elevated privileges using safe privilege escalation
func (m *UnixPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) (err error) {
	start := time.Now()

	// Perform privilege escalation
	if err := m.escalatePrivileges(ctx, elevationCtx); err != nil {
		m.metrics.RecordElevationFailure(err)
		return fmt.Errorf("privilege escalation failed: %w", err)
	}

	// Single defer for both privilege restoration and panic handling
	defer func() {
		var panicValue any
		var context string

		// Detect panic
		if r := recover(); r != nil {
			panicValue = r
			context = fmt.Sprintf("after panic: %v", r)
			m.logger.Error("Panic occurred during privileged operation, attempting privilege restoration",
				"panic", r,
				"original_uid", m.originalUID)
		} else {
			context = "normal execution"
		}

		// Restore privileges (always executed)
		if err := m.restorePrivileges(); err != nil {
			// Privilege restoration failure is critical security risk - terminate immediately
			m.emergencyShutdown(err, context)
		} else if panicValue == nil && err == nil {
			// Record metrics on success
			duration := time.Since(start)
			m.metrics.RecordElevationSuccess(duration)
		}

		// Re-panic if necessary
		if panicValue != nil {
			panic(panicValue)
		}
	}()

	return fn()
}

// escalatePrivileges performs the actual privilege escalation (private method)
func (m *UnixPrivilegeManager) escalatePrivileges(_ context.Context, elevationCtx ElevationContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.IsPrivilegedExecutionSupported() {
		return fmt.Errorf("%w: binary not configured with setuid", ErrPrivilegedExecutionNotAvailable)
	}

	elevationCtx.StartTime = time.Now()
	elevationCtx.OriginalUID = m.originalUID
	elevationCtx.TargetUID = 0

	if err := syscall.Seteuid(0); err != nil {
		return &Error{
			Operation:   elevationCtx.Operation,
			CommandName: elevationCtx.CommandName,
			OriginalUID: elevationCtx.OriginalUID,
			TargetUID:   elevationCtx.TargetUID,
			SyscallErr:  err,
			Timestamp:   time.Now(),
		}
	}

	m.logger.Info("Privileges elevated",
		"operation", elevationCtx.Operation,
		"command", elevationCtx.CommandName,
		"original_uid", elevationCtx.OriginalUID)

	return nil
}

// restorePrivileges restores original privileges (private method)
func (m *UnixPrivilegeManager) restorePrivileges() error {
	if err := syscall.Seteuid(m.originalUID); err != nil {
		return err
	}

	m.logger.Info("Privileges restored",
		"restored_uid", m.originalUID)

	return nil
}

// emergencyShutdown handles critical privilege restoration failures
func (m *UnixPrivilegeManager) emergencyShutdown(restoreErr error, context string) {
	// Record detailed error information (ensure logging to multiple destinations)
	criticalMsg := fmt.Sprintf("CRITICAL SECURITY FAILURE: Privilege restoration failed during %s", context)

	// Log to structured logger
	m.logger.Error(criticalMsg,
		"error", restoreErr,
		"original_uid", m.originalUID,
		"current_uid", os.Getuid(),
		"current_euid", os.Geteuid(),
		"timestamp", time.Now().UTC(),
		"process_id", os.Getpid(),
	)

	// Also log to system logger (for external forwarding via rsyslog etc.)
	if syslogWriter, err := syslog.New(syslog.LOG_ERR, "go-safe-cmd-runner"); err == nil {
		_ = syslogWriter.Err(fmt.Sprintf("%s: %v (PID: %d, UID: %d->%d)",
			criticalMsg, restoreErr, os.Getpid(), m.originalUID, os.Geteuid()))
		_ = syslogWriter.Close()
	}

	// Also log to stderr as last resort
	fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", criticalMsg, restoreErr)

	// Immediately terminate process (skip defer processing)
	os.Exit(1)
}

// IsPrivilegedExecutionSupported checks if privileged execution is available on this system
func (m *UnixPrivilegeManager) IsPrivilegedExecutionSupported() bool {
	return m.isSetuid
}

// GetCurrentUID returns the current effective user ID
func (m *UnixPrivilegeManager) GetCurrentUID() int {
	return syscall.Geteuid()
}

// GetOriginalUID returns the original user ID before any privilege elevation
func (m *UnixPrivilegeManager) GetOriginalUID() int {
	return m.originalUID
}

// HealthCheck verifies that privilege escalation works correctly
func (m *UnixPrivilegeManager) HealthCheck(ctx context.Context) error {
	if !m.IsPrivilegedExecutionSupported() {
		return ErrPrivilegedExecutionNotAvailable
	}

	// Test privilege elevation and restoration
	testCtx := ElevationContext{
		Operation:   OperationHealthCheck,
		CommandName: "health_check",
	}

	return m.WithPrivileges(ctx, testCtx, func() error {
		// Verify we're actually running as root
		if syscall.Geteuid() != 0 {
			return fmt.Errorf("%w: still running as uid %d", ErrPrivilegeElevationFailed, syscall.Geteuid())
		}
		return nil
	})
}

// GetMetrics returns a snapshot of current privilege operation metrics
func (m *UnixPrivilegeManager) GetMetrics() Metrics {
	return m.metrics.GetSnapshot()
}
