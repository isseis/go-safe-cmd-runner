//go:build !windows

package privilege

import (
	"errors"
	"fmt"
	"log/slog"
	"log/syslog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// UnixPrivilegeManager implements privilege management for Unix systems using setuid
type UnixPrivilegeManager struct {
	logger             *slog.Logger
	originalUID        int
	privilegeSupported bool
	metrics            Metrics
	mu                 sync.Mutex
}

func newPlatformManager(logger *slog.Logger) Manager {
	return &UnixPrivilegeManager{
		logger:             logger,
		originalUID:        syscall.Getuid(),
		privilegeSupported: isPrivilegeExecutionSupported(logger),
	}
}

// WithPrivileges executes a function with elevated privileges using safe privilege escalation
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
	// Lock for the entire duration of the privileged operation to prevent race conditions
	m.mu.Lock()
	defer m.mu.Unlock()

	start := time.Now()

	// Perform privilege escalation
	if err := m.escalatePrivileges(elevationCtx); err != nil {
		m.metrics.RecordElevationFailure(err)
		return fmt.Errorf("privilege escalation failed: %w", err)
	}

	// Single defer for both privilege restoration and panic handling
	defer func() {
		var panicValue any
		var shutdownContext string

		// Detect panic
		if r := recover(); r != nil {
			panicValue = r
			shutdownContext = fmt.Sprintf("after panic: %v", r)
			m.logger.Error("Panic occurred during privileged operation, attempting privilege restoration",
				"panic", r,
				"original_uid", m.originalUID)
		} else {
			shutdownContext = "normal execution"
		}

		// Calculate duration before restoring privileges to get accurate elevation time
		var duration time.Duration
		if panicValue == nil {
			duration = time.Since(start)
		}

		// Restore privileges (always executed)
		if err := m.restorePrivileges(); err != nil {
			// Privilege restoration failure is critical security risk - terminate immediately
			m.emergencyShutdown(err, shutdownContext)
		} else if panicValue == nil {
			// Record metrics on success
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
// Note: This method assumes the caller (WithPrivileges) has already acquired the mutex lock
func (m *UnixPrivilegeManager) escalatePrivileges(elevationCtx runnertypes.ElevationContext) error {
	if !m.IsPrivilegedExecutionSupported() {
		return fmt.Errorf("%w: privilege execution not supported", runnertypes.ErrPrivilegedExecutionNotAvailable)
	}

	elevationCtx.OriginalUID = m.originalUID
	elevationCtx.TargetUID = 0

	// For native root execution, no seteuid call is needed
	if m.originalUID == 0 {
		m.logger.Info("Native root execution - no privilege escalation needed",
			"operation", elevationCtx.Operation,
			"command", elevationCtx.CommandName,
			"original_uid", elevationCtx.OriginalUID)
		return nil
	}

	// For setuid binary execution, perform seteuid
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
// Note: This method assumes the caller (WithPrivileges) has already acquired the mutex lock
func (m *UnixPrivilegeManager) restorePrivileges() error {
	// For native root execution, no privilege restoration is needed
	if m.originalUID == 0 {
		m.logger.Info("Native root execution - no privilege restoration needed",
			"original_uid", m.originalUID)
		return nil
	}

	// For setuid binary execution, restore privileges
	if err := syscall.Seteuid(m.originalUID); err != nil {
		return err
	}

	m.logger.Info("Privileges restored",
		"restored_uid", m.originalUID)

	return nil
}

// emergencyShutdown handles critical privilege restoration failures
func (m *UnixPrivilegeManager) emergencyShutdown(restoreErr error, shutdownContext string) {
	// Record detailed error information (ensure logging to multiple destinations)
	criticalMsg := fmt.Sprintf("CRITICAL SECURITY FAILURE: Privilege restoration failed during %s", shutdownContext)

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
	progName := "go-safe-cmd-runner" // Default fallback
	if execPath, err := os.Executable(); err == nil {
		progName = filepath.Base(execPath)
	}

	// Try to log to syslog, but handle errors appropriately
	syslogWriter, err := syslog.New(syslog.LOG_ERR, progName)
	if err != nil {
		m.logger.Error("Failed to initialize syslog for critical error logging",
			"error", err,
			"original_uid", m.originalUID,
			"current_uid", os.Getuid(),
			"current_euid", os.Geteuid(),
			"timestamp", time.Now().UTC(),
			"process_id", os.Getpid(),
		)
		// Optionally log the syslog initialization failure to stderr as fallback
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize syslog: %v\n", err)
	} else {
		// Log the critical message to syslog
		if err := syslogWriter.Err(fmt.Sprintf("%s: %v (PID: %d, UID: %d->%d)",
			criticalMsg, restoreErr, os.Getpid(), m.originalUID, os.Geteuid())); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: Failed to write to syslog: %v\n", err)
		}

		// Close the syslog writer and check for errors
		if err := syslogWriter.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: Failed to close syslog writer: %v\n", err)
		}
	}

	// Also log to stderr as last resort
	fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", criticalMsg, restoreErr)

	// Immediately terminate process (skip defer processing)
	os.Exit(1)
}

// IsPrivilegedExecutionSupported checks if privileged execution is available on this system
func (m *UnixPrivilegeManager) IsPrivilegedExecutionSupported() bool {
	return m.privilegeSupported
}

// GetCurrentUID returns the current effective user ID
func (m *UnixPrivilegeManager) GetCurrentUID() int {
	return syscall.Geteuid()
}

// isPrivilegeExecutionSupported checks if privileged execution is supported
// This includes both setuid binaries and native root execution
func isPrivilegeExecutionSupported(logger *slog.Logger) bool {
	originalUID := syscall.Getuid()
	effectiveUID := syscall.Geteuid()

	// Case 1: Native root execution (both real and effective UID are 0)
	if originalUID == 0 && effectiveUID == 0 {
		logger.Info("Privilege execution supported: native root execution",
			"original_uid", originalUID,
			"effective_uid", effectiveUID,
			"execution_mode", "native_root")
		return true
	}

	// Case 2: Setuid binary execution (check file system properties)
	return isRootOwnedSetuidBinary(logger)
}

// isRootOwnedSetuidBinary checks if the current binary has the setuid bit set and is owned by root
// This provides more robust detection than checking runtime UID/EUID which
// can be altered by previous seteuid() calls
func isRootOwnedSetuidBinary(logger *slog.Logger) bool {
	// Get the path to the current executable
	execPath, err := os.Executable()
	if err != nil {
		logger.Warn("Failed to get executable path for setuid detection",
			"error", err)
		return false
	}

	// Get file information
	fileInfo, err := os.Stat(execPath)
	if err != nil {
		logger.Warn("Failed to stat executable for setuid detection",
			"path", execPath,
			"error", err)
		return false
	}

	// Check if the setuid bit is set
	hasSetuidBit := fileInfo.Mode()&os.ModeSetuid != 0

	// Check if the file is owned by root (UID 0)
	// This is essential for setuid to work - only root-owned setuid binaries can escalate privileges
	var isOwnedByRoot bool
	if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
		isOwnedByRoot = stat.Uid == 0
	} else {
		logger.Warn("Failed to get file ownership information for setuid detection",
			"path", execPath)
		return false
	}

	// Additional validation: ensure we can actually escalate privileges
	// This catches cases where setuid bit is set but we're already running as root
	originalUID := syscall.Getuid()
	effectiveUID := syscall.Geteuid()

	// True setuid scenario: setuid bit + root ownership + non-root real UID
	isValidSetuid := hasSetuidBit && isOwnedByRoot && originalUID != 0

	if isValidSetuid {
		logger.Info("Privilege execution supported: setuid binary execution",
			"executable_path", execPath,
			"has_setuid_bit", hasSetuidBit,
			"is_owned_by_root", isOwnedByRoot,
			"original_uid", originalUID,
			"effective_uid", effectiveUID,
			"execution_mode", "setuid_binary")
	} else {
		logger.Info("Setuid binary detection completed - not supported",
			"executable_path", execPath,
			"has_setuid_bit", hasSetuidBit,
			"is_owned_by_root", isOwnedByRoot,
			"original_uid", originalUID,
			"effective_uid", effectiveUID,
			"reason", "missing_required_conditions")
	}

	return isValidSetuid
}

// GetOriginalUID returns the original user ID before any privilege elevation
func (m *UnixPrivilegeManager) GetOriginalUID() int {
	return m.originalUID
}

// GetMetrics returns a snapshot of current privilege operation metrics
func (m *UnixPrivilegeManager) GetMetrics() Metrics {
	return m.metrics.GetSnapshot()
}

// WithUserGroup executes a function with specified user and group privileges
func (m *UnixPrivilegeManager) WithUserGroup(user, group string, fn func() error) (err error) {
	// Lock for the entire duration of the privileged operation
	m.mu.Lock()
	defer m.mu.Unlock()

	start := time.Now()

	// Get current UID/GID before any changes
	originalUID := syscall.Getuid()
	originalGID := syscall.Getgid()

	// Perform user/group changes
	if err := m.changeUserGroup(user, group); err != nil {
		m.metrics.RecordElevationFailure(err)
		return fmt.Errorf("user/group change failed: %w", err)
	}

	// Single defer for both privilege restoration and panic handling
	defer func() {
		if restoreErr := m.restoreUserGroup(originalUID, originalGID); restoreErr != nil {
			m.logger.Error("Critical failure in user/group privilege restoration",
				"restore_error", restoreErr,
				"original_uid", originalUID,
				"original_gid", originalGID)
			m.emergencyShutdown(restoreErr, "user_group_restore")
		}

		// Record metrics after restoration
		duration := time.Since(start)
		if err != nil {
			m.metrics.RecordElevationFailure(err)
		} else {
			m.metrics.RecordElevationSuccess(duration)
		}
	}()

	// Execute the function with changed privileges
	err = fn()
	if err != nil {
		m.logger.Error("Function execution failed with user/group privileges",
			"error", err,
			"user", user,
			"group", group)
	}

	return err
}

// IsUserGroupSupported checks if user/group privilege changes are supported
func (m *UnixPrivilegeManager) IsUserGroupSupported() bool {
	// User/group changes are supported on Unix systems when running with appropriate privileges
	return m.privilegeSupported
}

// changeUserGroup changes the effective user and group IDs
func (m *UnixPrivilegeManager) changeUserGroup(user, group string) error {
	// TODO: Implement user/group name to UID/GID resolution
	// For now, this is a placeholder that would need to:
	// 1. Look up user/group names to get UID/GID
	// 2. Call setegid/seteuid system calls
	// 3. Handle permission validation

	m.logger.Info("User/group change requested",
		"user", user,
		"group", group)

	// Placeholder implementation - needs proper user/group lookup
	return fmt.Errorf("%w: user=%s, group=%s", ErrUserGroupChangeNotImplemented, user, group)
}

// restoreUserGroup restores the original user and group IDs
func (m *UnixPrivilegeManager) restoreUserGroup(originalUID, originalGID int) error {
	// Restore group first, then user (reverse order of setting)
	if err := syscall.Setegid(originalGID); err != nil {
		return fmt.Errorf("failed to restore group ID to %d: %w", originalGID, err)
	}

	if err := syscall.Seteuid(originalUID); err != nil {
		return fmt.Errorf("failed to restore user ID to %d: %w", originalUID, err)
	}

	m.logger.Info("User/group privileges restored",
		"restored_uid", originalUID,
		"restored_gid", originalGID)

	return nil
}

// ErrUserGroupChangeNotImplemented indicates that user/group change functionality is not yet implemented.
var ErrUserGroupChangeNotImplemented = errors.New("user/group change not yet implemented")
