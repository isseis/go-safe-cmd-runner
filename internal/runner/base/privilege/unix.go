//go:build !windows

package privilege

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"syscall"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// ErrInsufficientPrivileges is returned when the user lacks sufficient privileges to change user/group.
var ErrInsufficientPrivileges = errors.New("insufficient privileges to change user/group")

// ErrUnsupportedOperationType is returned when an unsupported operation type is encountered
var ErrUnsupportedOperationType = errors.New("unsupported operation type")

// ErrIdentityLeak is returned when effective UID/GID do not match real UID/GID after privilege restoration.
var ErrIdentityLeak = errors.New("privilege identity leak detected")

// ErrSavedSetNotSupported is returned by readSavedIDs on platforms where this
// project does not implement reading the saved-set-uid/gid (non-Linux). The
// caller must skip the saved-set invariant check when this error is returned.
var ErrSavedSetNotSupported = errors.New("saved-set IDs not supported on this platform")

// UnixPrivilegeManager implements privilege management for Unix systems using setuid
type UnixPrivilegeManager struct {
	logger             *slog.Logger
	originalUID        int
	privilegeSupported bool
	// inPrivilegedWindow rejects reentrant WithPrivileges calls on this manager
	// instance; it does not catch reentry through a second manager instance,
	// since the euid it guards is process-wide. Unsynchronized: only one
	// goroutine may call WithPrivileges, so concurrent callers would race and
	// could both read false, defeating the guard.
	inPrivilegedWindow bool
	// osExit is a function for os.Exit to enable testing of emergencyShutdown
	osExit func(code int)
	// identityVerifier checks that EUID==UID and EGID==GID; injectable for testing
	identityVerifier func() error
	// readSavedIDs reads the process's saved-set-uid/gid; injectable for testing
	readSavedIDs func() (suid, sgid int, err error)
}

// getReadSavedIDs returns m.readSavedIDs if injected for testing, otherwise
// the package-level readSavedIDs implementation. Manager literals constructed
// without setting the field (e.g. in existing tests) fall back to the real
// implementation rather than panicking on a nil func field.
func (m *UnixPrivilegeManager) getReadSavedIDs() func() (suid, sgid int, err error) {
	if m.readSavedIDs != nil {
		return m.readSavedIDs
	}
	return readSavedIDs
}

func newPlatformManager(logger *slog.Logger) Manager {
	return &UnixPrivilegeManager{
		logger:             logger,
		originalUID:        syscall.Getuid(),
		privilegeSupported: isPrivilegeExecutionSupported(logger),
		osExit:             os.Exit,
		identityVerifier:   defaultIdentityVerifier,
		readSavedIDs:       readSavedIDs,
	}
}

// defaultIdentityVerifier checks that EUID == UID and EGID == GID.
// This is the security invariant that must hold after every privilege restoration:
// the process must not carry elevated identity between operations.
func defaultIdentityVerifier() error {
	if euid, uid := syscall.Geteuid(), syscall.Getuid(); euid != uid {
		return fmt.Errorf("effective UID %d does not match real UID %d after privilege restoration: %w", euid, uid, ErrIdentityLeak)
	}
	if egid, gid := syscall.Getegid(), syscall.Getgid(); egid != gid {
		return fmt.Errorf("effective GID %d does not match real GID %d after privilege restoration: %w", egid, gid, ErrIdentityLeak)
	}
	return nil
}

// WithPrivileges executes fn under the privilege state required by elevationCtx.Operation.
//
// For both OperationUserGroupExecution and OperationFileValidation, this package
// only escalates to root and restores afterwards; it never reads or resolves
// RunAsUser/RunAsGroup. Switching to the target user is the executor's job: it
// builds a syscall.Credential the kernel applies at execve time.
//
// The window is not serialized: while it's open, the process-wide euid is
// raised for every goroutine, including os/exec's copy goroutines for
// non-*os.File writers. This is an unresolved design issue: introducing
// parallel execution needs a separate design, not a lock here.
//
// Not reentrant: a nested call on the same manager from within fn returns
// ErrReentrantPrivilegeCall instead of opening a second window.
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
	if m.inPrivilegedWindow {
		return ErrReentrantPrivilegeCall
	}
	m.inPrivilegedWindow = true
	defer func() { m.inPrivilegedWindow = false }()

	execCtx, err := m.prepareExecution(elevationCtx)
	if err != nil {
		return err
	}

	if err := m.performElevation(execCtx); err != nil {
		return err
	}

	defer m.handleCleanup(execCtx)
	m.logger.Debug("Executing privileged operation callback", "operation", execCtx.elevationCtx.Operation, "command", execCtx.elevationCtx.CommandName)
	fnErr := fn()
	m.logger.Debug("Privileged operation callback completed", "operation", execCtx.elevationCtx.Operation, "command", execCtx.elevationCtx.CommandName, "error", fnErr)
	return fnErr
}

// executionContext holds context for privilege execution
type executionContext struct {
	elevationCtx runnertypes.ElevationContext
	// needsPrivilegeEscalation indicates whether system-level privilege escalation (setuid to root) is required.
	// This is needed to gain administrative privileges for operations like file validation or user switching.
	// When true, escalatePrivileges() will call syscall.Seteuid(0) to become root. The parent process never
	// changes its identity to the target user; the executor applies that via syscall.Credential at execve time.
	needsPrivilegeEscalation bool
	originalSUID             int
	originalSGID             int
}

// prepareExecution validates and prepares the execution context
func (m *UnixPrivilegeManager) prepareExecution(elevationCtx runnertypes.ElevationContext) (*executionContext, error) {
	suid, sgid, err := m.getReadSavedIDs()()
	if err != nil && !errors.Is(err, ErrSavedSetNotSupported) {
		return nil, fmt.Errorf("failed to read saved-set IDs before execution: %w", err)
	}

	// On platforms where reading saved-set IDs is not implemented (non-Linux),
	// use sentinel values so the saved-set invariant check is structurally skipped.
	if errors.Is(err, ErrSavedSetNotSupported) {
		suid = -1
		sgid = -1
	}

	execCtx := &executionContext{
		elevationCtx: elevationCtx,
		originalSUID: suid,
		originalSGID: sgid,
	}

	switch elevationCtx.Operation {
	case runnertypes.OperationUserGroupExecution, runnertypes.OperationFileValidation:
		execCtx.needsPrivilegeEscalation = true
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedOperationType, elevationCtx.Operation)
	}

	return execCtx, nil
}

// performElevation performs the actual privilege escalation.
//
// Escalation is deliberately the last fallible step: WithPrivileges only registers
// the deferred restore-and-verify once this function has returned successfully, so
// anything that failed after a successful escalation would return with EUID 0 still
// held and no restoration. Keeping escalation last makes that unreachable by
// construction rather than by the current operation mapping, which is what the
// removed post-escalation rollback block used to compensate for.
func (m *UnixPrivilegeManager) performElevation(execCtx *executionContext) error {
	if execCtx.needsPrivilegeEscalation {
		if err := m.escalatePrivileges(execCtx.elevationCtx); err != nil {
			return fmt.Errorf("privilege escalation failed: %w", err)
		}
	}

	return nil
}

// handleCleanup recovers from a panic in the callback, restores privileges, verifies identity,
// and then re-raises the panic, if any.
func (m *UnixPrivilegeManager) handleCleanup(execCtx *executionContext) {
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

	m.restorePrivilegesAndVerify(execCtx, shutdownContext)

	if panicValue != nil {
		panic(panicValue)
	}
}

// restorePrivilegesAndVerify restores the original privileges and verifies that no elevated identity leaked.
func (m *UnixPrivilegeManager) restorePrivilegesAndVerify(execCtx *executionContext, shutdownContext string) {
	// Note: no branch restores the effective group ID here. This package only escalates
	// and restores the effective UID, so there is nothing else to restore.
	if execCtx.needsPrivilegeEscalation {
		if err := m.restorePrivileges(); err != nil {
			m.emergencyShutdown(err, shutdownContext)
		}
	}

	// Defense-in-depth: verify EUID==UID and EGID==GID after every privilege operation that
	// escalated. This is an independent check of the privilege manager's own restoration
	// logic and catches any leakage regardless of which restore path ran.
	// Only privilege escalation changes identity, so escalation alone gates verification.
	if execCtx.needsPrivilegeEscalation {
		if err := m.identityVerifier(); err != nil {
			m.emergencyShutdown(err, fmt.Sprintf("identity_verification_failure_%s", shutdownContext))
		}

		// Verify saved-set-uid/gid are unchanged since capture. This is a stronger
		// invariant than EUID==UID: it catches cases where EUID was restored but
		// the saved-set was corrupted (e.g. by a partial seteuid that left the
		// saved-set as the previous effective UID). The saved-set should only
		// change when the process explicitly calls setresuid/setresgid, so any
		// mismatch after restore indicates a privilege leak.
		//
		// On platforms where reading saved-set IDs is not implemented (non-Linux),
		// the capture uses sentinel value -1 and readSavedIDs returns ErrSavedSetNotSupported.
		// The check is structurally skipped in that case via the originalSUID >= 0
		// gate, rather than relying on both sides returning the same constant.
		if execCtx.originalSUID >= 0 {
			suid, sgid, err := m.getReadSavedIDs()()
			if err != nil {
				m.emergencyShutdown(fmt.Errorf("failed to read saved-set IDs after restore: %w", err),
					fmt.Sprintf("saved_set_read_failure_%s", shutdownContext))
			}
			if suid != execCtx.originalSUID || sgid != execCtx.originalSGID {
				err := fmt.Errorf("saved-set-uid/gid changed after restore: "+
					"original suid=%d, sgid=%d; post-restore suid=%d, sgid=%d: %w",
					execCtx.originalSUID, execCtx.originalSGID, suid, sgid, ErrIdentityLeak)
				m.emergencyShutdown(err, fmt.Sprintf("saved_set_identity_verification_failure_%s", shutdownContext))
			}
		}
	}
}

// escalatePrivileges performs the actual privilege escalation (private method)
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

	// Also log to stderr as last resort
	fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", criticalMsg, restoreErr)

	// Immediately terminate process (skip defer processing)
	m.osExit(1)
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
