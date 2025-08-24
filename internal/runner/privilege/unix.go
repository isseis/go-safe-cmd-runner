//go:build !windows

package privilege

import (
	"errors"
	"fmt"
	"log/slog"
	"log/syslog"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ErrInsufficientPrivileges is returned when the user lacks sufficient privileges to change user/group.
var ErrInsufficientPrivileges = errors.New("insufficient privileges to change user/group")

// ErrUnsupportedOperationType is returned when an unsupported operation type is encountered
var ErrUnsupportedOperationType = errors.New("unsupported operation type")

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
	m.mu.Lock()
	defer m.mu.Unlock()

	execCtx, err := m.prepareExecution(elevationCtx)
	if err != nil {
		m.metrics.RecordElevationFailure(err)
		return err
	}

	if err := m.performElevation(execCtx); err != nil {
		m.metrics.RecordElevationFailure(err)
		return err
	}

	defer m.handleCleanupAndMetrics(execCtx)
	return fn()
}

// executionContext holds context for privilege execution
type executionContext struct {
	elevationCtx runnertypes.ElevationContext
	// needsPrivilegeEscalation indicates whether system-level privilege escalation (setuid to root) is required.
	// This is needed to gain administrative privileges for operations like file validation or user switching.
	// When true, escalatePrivileges() will call syscall.Seteuid(0) to become root.
	needsPrivilegeEscalation bool
	// needsUserGroupChange indicates whether user/group identity change is required.
	// This controls whether changeUserGroupInternal() should be called to validate or switch to the target user/group.
	// IMPORTANT: This operation requires root privileges (needsPrivilegeEscalation=true) to be performed first,
	// because only root can change effective UID/GID to arbitrary values via syscall.Seteuid()/Setegid().
	// The typical flow is: current_user -> root (via escalatePrivileges) -> target_user (via changeUserGroupInternal).
	needsUserGroupChange bool
	originalEUID         int
	originalEGID         int
	start                time.Time
}

// prepareExecution validates and prepares the execution context
func (m *UnixPrivilegeManager) prepareExecution(elevationCtx runnertypes.ElevationContext) (*executionContext, error) {
	execCtx := &executionContext{
		elevationCtx: elevationCtx,
		originalEUID: syscall.Geteuid(),
		originalEGID: syscall.Getegid(),
		start:        time.Now(),
	}

	switch elevationCtx.Operation {
	case runnertypes.OperationUserGroupExecution:
		execCtx.needsPrivilegeEscalation = true
		execCtx.needsUserGroupChange = true
	case runnertypes.OperationUserGroupDryRun:
		execCtx.needsPrivilegeEscalation = false
		execCtx.needsUserGroupChange = true
	case runnertypes.OperationFileValidation:
		execCtx.needsPrivilegeEscalation = true
		execCtx.needsUserGroupChange = false
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedOperationType, elevationCtx.Operation)
	}

	return execCtx, nil
}

// performElevation performs the actual privilege escalation and user/group changes
func (m *UnixPrivilegeManager) performElevation(execCtx *executionContext) error {
	if execCtx.needsPrivilegeEscalation {
		if err := m.escalatePrivileges(execCtx.elevationCtx); err != nil {
			return fmt.Errorf("privilege escalation failed: %w", err)
		}
	}

	if execCtx.needsUserGroupChange {
		isDryRun := execCtx.elevationCtx.Operation == runnertypes.OperationUserGroupDryRun
		if err := m.changeUserGroupInternal(execCtx.elevationCtx.RunAsUser, execCtx.elevationCtx.RunAsGroup, isDryRun); err != nil {
			if execCtx.needsPrivilegeEscalation {
				if restoreErr := m.restorePrivileges(); restoreErr != nil {
					m.emergencyShutdown(restoreErr, "user_group_change_failure")
				}
			}
			return fmt.Errorf("user/group change failed: %w", err)
		}
	}

	return nil
}

// handleCleanupAndMetrics handles panic recovery, cleanup, and metrics recording
func (m *UnixPrivilegeManager) handleCleanupAndMetrics(execCtx *executionContext) {
	var panicValue any
	var shutdownContext string

	if r := recover(); r != nil {
		panicValue = r
		shutdownContext = fmt.Sprintf("after panic: %v", r)
		m.logger.Error("Panic occurred during privileged operation, attempting privilege restoration",
			"panic", r, "original_uid", m.originalUID)
	} else {
		shutdownContext = "normal execution"
	}

	var duration time.Duration
	if panicValue == nil {
		duration = time.Since(execCtx.start)
	}

	m.restorePrivilegesAndMetrics(execCtx, panicValue, shutdownContext, duration)

	if panicValue != nil {
		panic(panicValue)
	}
}

// restorePrivilegesAndMetrics handles privilege restoration and metrics recording
func (m *UnixPrivilegeManager) restorePrivilegesAndMetrics(execCtx *executionContext, panicValue any, shutdownContext string, duration time.Duration) {
	if execCtx.needsUserGroupChange && execCtx.elevationCtx.Operation != runnertypes.OperationUserGroupDryRun {
		if err := m.restoreUserGroupInternal(execCtx.originalEGID); err != nil {
			m.emergencyShutdown(err, fmt.Sprintf("user_group_restore_failure_%s", shutdownContext))
		}
	}

	if execCtx.needsPrivilegeEscalation {
		if err := m.restorePrivileges(); err != nil {
			m.emergencyShutdown(err, shutdownContext)
		} else if panicValue == nil {
			m.metrics.RecordElevationSuccess(duration)
		}
	} else if panicValue == nil && (execCtx.needsPrivilegeEscalation || execCtx.needsUserGroupChange) {
		m.metrics.RecordElevationSuccess(duration)
	}
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

	m.logger.Info("Privileges fully restored to original state",
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

// IsUserGroupSupported checks if user/group privilege changes are supported
func (m *UnixPrivilegeManager) IsUserGroupSupported() bool {
	// User/group changes are supported on Unix systems when running with appropriate privileges
	return m.privilegeSupported
}

// changeUserGroupDryRun validates user/group configuration without making actual changes
func (m *UnixPrivilegeManager) changeUserGroupDryRun(userName, groupName string) error {
	return m.changeUserGroupInternal(userName, groupName, true)
}

// changeUserGroupInternal implements the core user/group change logic with optional dry-run mode
// Note: This method assumes the caller has already acquired appropriate privileges
func (m *UnixPrivilegeManager) changeUserGroupInternal(userName, groupName string, dryRun bool) error {
	m.logger.Info("User/group change requested",
		"user", userName,
		"group", groupName,
		"dry_run", dryRun)

	// Resolve user name to UID
	var targetUID int
	if userName != "" {
		userInfo, err := user.Lookup(userName)
		if err != nil {
			return fmt.Errorf("failed to lookup user %s: %w", userName, err)
		}

		uid, err := strconv.Atoi(userInfo.Uid)
		if err != nil {
			return fmt.Errorf("invalid UID %s for user %s: %w", userInfo.Uid, userName, err)
		}
		targetUID = uid
	} else {
		// If no user specified, keep current effective user
		targetUID = syscall.Geteuid()
	}

	// Resolve group name to GID
	var targetGID int
	if groupName != "" {
		groupInfo, err := user.LookupGroup(groupName)
		if err != nil {
			return fmt.Errorf("failed to lookup group %s: %w", groupName, err)
		}

		gid, err := strconv.Atoi(groupInfo.Gid)
		if err != nil {
			return fmt.Errorf("invalid GID %s for group %s: %w", groupInfo.Gid, groupName, err)
		}
		targetGID = gid
	} else if userName != "" {
		// If user is specified but group is not, default to user's primary group
		userInfo, err := user.Lookup(userName)
		if err != nil {
			return fmt.Errorf("failed to lookup user %s for primary group: %w", userName, err)
		}

		gid, err := strconv.Atoi(userInfo.Gid)
		if err != nil {
			return fmt.Errorf("invalid primary GID %s for user %s: %w", userInfo.Gid, userName, err)
		}
		targetGID = gid
		m.logger.Info("Defaulting to user's primary group",
			"user", userName,
			"primary_gid", targetGID)
	} else {
		// If neither user nor group specified, keep current effective group
		targetGID = syscall.Getegid()
	}

	if dryRun {
		m.logger.Info("Dry-run mode: would change user/group privileges",
			"user", userName,
			"group", groupName,
			"target_uid", targetUID,
			"target_gid", targetGID,
			"current_uid", syscall.Getuid(),
			"current_gid", syscall.Getgid())
		return nil
	}

	// Set group first, then user (standard practice)
	if err := syscall.Setegid(targetGID); err != nil {
		return fmt.Errorf("failed to set effective group ID to %d (group %s): %w", targetGID, groupName, err)
	}

	if err := syscall.Seteuid(targetUID); err != nil {
		// Try to restore original GID on failure
		if restoreErr := syscall.Setegid(syscall.Getegid()); restoreErr != nil {
			m.logger.Error("Failed to restore GID after UID change failure",
				"restore_error", restoreErr)
		}
		return fmt.Errorf("failed to set effective user ID to %d (user %s): %w", targetUID, userName, err)
	}

	m.logger.Info("User/group privileges changed successfully",
		"user", userName,
		"group", groupName,
		"target_uid", targetUID,
		"target_gid", targetGID)

	return nil
}

// restoreUserGroupInternal restores the original effective group ID only
// Note: User ID restoration is handled by restorePrivileges() to avoid conflicts
func (m *UnixPrivilegeManager) restoreUserGroupInternal(originalEGID int) error {
	// Only restore group ID - user ID will be restored by restorePrivileges()
	if err := syscall.Setegid(originalEGID); err != nil {
		return fmt.Errorf("failed to restore effective group ID to %d: %w", originalEGID, err)
	}

	m.logger.Info("User/group privileges partially restored (group only)",
		"restored_egid", originalEGID,
		"note", "user ID will be restored by privilege restoration")

	return nil
}
