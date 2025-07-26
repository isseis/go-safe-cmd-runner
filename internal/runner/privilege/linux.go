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

// LinuxPrivilegeManager implements privilege management for Linux/Unix systems using setuid
type LinuxPrivilegeManager struct {
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

	return &LinuxPrivilegeManager{
		logger:      logger,
		originalUID: originalUID,
		originalGID: syscall.Getgid(),
		isSetuid:    effectiveUID == 0 && originalUID != 0,
	}
}

// WithPrivileges executes a function with elevated privileges using safe privilege escalation
func (m *LinuxPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) (err error) {
	start := time.Now()

	// 権限昇格
	if err := m.escalatePrivileges(ctx, elevationCtx); err != nil {
		m.metrics.RecordElevationFailure(err)
		return fmt.Errorf("privilege escalation failed: %w", err)
	}

	// 単一のdefer文で権限復帰とpanic処理を統合
	defer func() {
		var panicValue interface{}
		var context string

		// panic検出
		if r := recover(); r != nil {
			panicValue = r
			context = fmt.Sprintf("after panic: %v", r)
			m.logger.Error("Panic occurred during privileged operation, attempting privilege restoration",
				"panic", r,
				"original_uid", m.originalUID)
		} else {
			context = "normal execution"
		}

		// 権限復帰実行（常に実行される）
		if err := m.restorePrivileges(); err != nil {
			// 権限復帰失敗は致命的セキュリティリスク - 即座に終了
			m.emergencyShutdown(err, context)
		} else if panicValue == nil && err == nil {
			// 成功時のメトリクス記録
			duration := time.Since(start)
			m.metrics.RecordElevationSuccess(duration)
		}

		// panic再発生（必要な場合のみ）
		if panicValue != nil {
			panic(panicValue)
		}
	}()

	return fn()
}

// escalatePrivileges performs the actual privilege escalation (private method)
func (m *LinuxPrivilegeManager) escalatePrivileges(_ context.Context, elevationCtx ElevationContext) error {
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
func (m *LinuxPrivilegeManager) restorePrivileges() error {
	if err := syscall.Seteuid(m.originalUID); err != nil {
		return err
	}

	m.logger.Info("Privileges restored",
		"restored_uid", m.originalUID)

	return nil
}

// emergencyShutdown handles critical privilege restoration failures
func (m *LinuxPrivilegeManager) emergencyShutdown(restoreErr error, context string) {
	// 詳細なエラー情報を記録（複数の出力先に確実に記録）
	criticalMsg := fmt.Sprintf("CRITICAL SECURITY FAILURE: Privilege restoration failed during %s", context)

	// 構造化ログに記録
	m.logger.Error(criticalMsg,
		"error", restoreErr,
		"original_uid", m.originalUID,
		"current_uid", os.Getuid(),
		"current_euid", os.Geteuid(),
		"timestamp", time.Now().UTC(),
		"process_id", os.Getpid(),
	)

	// システムログにも記録（rsyslog等による外部転送対応）
	if syslogWriter, err := syslog.New(syslog.LOG_ERR, "go-safe-cmd-runner"); err == nil {
		_ = syslogWriter.Err(fmt.Sprintf("%s: %v (PID: %d, UID: %d->%d)",
			criticalMsg, restoreErr, os.Getpid(), m.originalUID, os.Geteuid()))
		_ = syslogWriter.Close()
	}

	// 標準エラー出力にも記録（最後の手段）
	fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", criticalMsg, restoreErr)

	// 即座にプロセス終了（defer処理をスキップ）
	os.Exit(1)
}

// IsPrivilegedExecutionSupported checks if privileged execution is available on this system
func (m *LinuxPrivilegeManager) IsPrivilegedExecutionSupported() bool {
	return m.isSetuid
}

// GetCurrentUID returns the current effective user ID
func (m *LinuxPrivilegeManager) GetCurrentUID() int {
	return syscall.Geteuid()
}

// GetOriginalUID returns the original user ID before any privilege elevation
func (m *LinuxPrivilegeManager) GetOriginalUID() int {
	return m.originalUID
}

// HealthCheck verifies that privilege escalation works correctly
func (m *LinuxPrivilegeManager) HealthCheck(ctx context.Context) error {
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
func (m *LinuxPrivilegeManager) GetMetrics() Metrics {
	return m.metrics.GetSnapshot()
}
